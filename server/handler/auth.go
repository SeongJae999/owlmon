package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/auth"
	"golang.org/x/crypto/bcrypt"
)

// userBackend — *db.UserStore가 만족. nil이면 환경변수 단일 계정 모드 (DB 없이 실행할 때).
type userBackend interface {
	ForLogin(ctx context.Context, username string) (hash, role string, disabled bool, err error)
	SetPassword(ctx context.Context, username, hash, keepSession string) error
	CreateSession(ctx context.Context, id, username string, expiresAt time.Time) error
	DeleteSession(ctx context.Context, id string) error
	TouchSession(ctx context.Context, id string) error
	ExtendSession(ctx context.Context, id string, expiresAt time.Time) error
}

type AuthHandler struct {
	username     string
	passwordHash string // bcrypt 해시
	jwtSecret    string
	auditStore   *audit.Store
	users        userBackend

	// brute-force 방어 (ISMS-P: 연속 실패 시 잠금). 계정+IP 단위라 남이 일부러 틀려도 정상 사용자는 잠기지 않음.
	// IP 단위 한도를 함께 두어 한 곳에서 여러 계정을 대입하는 것도 막음
	mu    sync.Mutex
	locks map[string]*loginLock
}

type loginLock struct {
	failures    int
	lastFailure time.Time
	lockedUntil time.Time
}

const (
	maxLoginFailures = 5
	maxIPFailures    = 20
	loginLockout     = 10 * time.Minute
	maxTrackedLocks  = 1000 // 없는 아이디를 무작위로 대입해 맵이 무한히 커지는 것 방지
)

func NewAuthHandler(username, passwordHash, jwtSecret string, auditStore *audit.Store) *AuthHandler {
	return &AuthHandler{
		username:     username,
		passwordHash: passwordHash,
		jwtSecret:    jwtSecret,
		auditStore:   auditStore,
		locks:        make(map[string]*loginLock),
	}
}

// WithUsers — 다중 계정 모드. 설정 후엔 환경변수 계정 대신 users 테이블로 로그인.
func (h *AuthHandler) WithUsers(u userBackend) *AuthHandler {
	h.users = u
	return h
}

func accountKey(user, ip string) string { return user + "\x00" + ip }

// lockRemaining — 계정+IP 잠금과 IP 잠금 중 더 오래 남은 시간
func (h *AuthHandler) lockRemaining(user, ip string) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	var left time.Duration
	for _, key := range []string{accountKey(user, ip), ip} {
		if l := h.locks[key]; l != nil {
			left = max(left, time.Until(l.lockedUntil))
		}
	}
	return left
}

// recordFailure — 실패 누적, 임계치 도달 시 잠금 시작. 잠금이 걸렸으면 true.
func (h *AuthHandler) recordFailure(user, ip string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	accountLocked := h.bump(accountKey(user, ip), maxLoginFailures)
	ipLocked := h.bump(ip, maxIPFailures)
	return accountLocked || ipLocked
}

func (h *AuthHandler) bump(key string, limit int) bool {
	now := time.Now()
	l := h.locks[key]
	if l == nil {
		if len(h.locks) >= maxTrackedLocks {
			for k, v := range h.locks {
				if now.After(v.lockedUntil) && now.Sub(v.lastFailure) > loginLockout {
					delete(h.locks, k)
				}
			}
		}
		l = &loginLock{}
		h.locks[key] = l
	}
	// 오래전 실패까지 쌓이면 학교처럼 IP 하나를 함께 쓰는 곳이 언젠가 잠기므로 시간이 지나면 초기화
	if now.Sub(l.lastFailure) > loginLockout {
		l.failures = 0
	}
	l.lastFailure = now
	l.failures++
	if l.failures >= limit {
		l.failures = 0
		l.lockedUntil = now.Add(loginLockout)
		return true
	}
	return false
}

// IP 카운터는 지우지 않음 — 아무 계정으로 한 번 로그인해 대입 한도를 초기화하지 못하게
func (h *AuthHandler) resetFailures(user, ip string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.locks, accountKey(user, ip))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// verify — 아이디·비밀번호 확인. 실패 시 감사 로그용 사유 반환.
func (h *AuthHandler) verify(ctx context.Context, username, password string) (role, reason string) {
	hash, role := h.passwordHash, auth.RoleAdmin
	if h.users != nil {
		var disabled bool
		var err error
		hash, role, disabled, err = h.users.ForLogin(ctx, username)
		if err != nil {
			return "", "unknown_user"
		}
		if disabled {
			return "", "disabled"
		}
	} else if username != h.username {
		return "", "unknown_user"
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "bad_password"
	}
	return role, ""
}

// Login은 아이디/비밀번호를 확인하고 JWT를 발급합니다.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}

	ip := audit.ClientIP(r)
	ua := r.UserAgent()
	save := func(action, result string, details map[string]any) {
		h.auditStore.Save(r.Context(), audit.Entry{
			Actor: req.Username, IP: ip, UserAgent: ua,
			Action: action, Result: result, Details: details,
		})
	}

	// 잠금 중에는 비밀번호가 맞아도 거부해야 무차별 대입이 무의미해짐
	if left := h.lockRemaining(req.Username, ip); left > 0 {
		save("auth.login", "failure", map[string]any{"reason": "locked"})
		mins := int(left.Minutes()) + 1
		http.Error(w, fmt.Sprintf("로그인 실패가 많아 잠겼습니다. %d분 후 다시 시도하세요", mins), http.StatusTooManyRequests)
		return
	}

	role, reason := h.verify(r.Context(), req.Username, req.Password)
	if reason != "" {
		save("auth.login", "failure", map[string]any{"reason": reason})
		if h.recordFailure(req.Username, ip) {
			save("auth.lockout", "failure", map[string]any{"minutes": int(loginLockout.Minutes())})
		}
		// 사유(없는 아이디/중지/비밀번호)를 구분해 알려주면 계정 존재 여부가 노출됨
		http.Error(w, "아이디 또는 비밀번호가 올바르지 않습니다", http.StatusUnauthorized)
		return
	}

	h.resetFailures(req.Username, ip)
	sessionID := rand.Text()
	ttl := auth.TokenTTL(role)
	if h.users != nil {
		if err := h.users.CreateSession(r.Context(), sessionID, req.Username, time.Now().Add(ttl)); err != nil {
			respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
			return
		}
	}
	token, err := auth.GenerateToken(req.Username, sessionID, h.jwtSecret, ttl)
	if err != nil {
		http.Error(w, "토큰 생성 실패", http.StatusInternalServerError)
		return
	}
	save("auth.login", "success", nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResponse{Token: token})
}

// Logout POST /api/auth/logout — 이 세션만 끊음 (같은 계정을 쓰는 다른 PC·상황판은 유지)
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if h.users != nil {
		if err := h.users.DeleteSession(r.Context(), auth.SessionFromContext(r.Context())); err != nil {
			respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Heartbeat POST /api/auth/heartbeat {active} — 화면이 1분마다 호출.
// 대시보드 자동 조회는 조작이 아니므로 사람이 조작했는지는 화면이 active로 알려줌
func (h *AuthHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if h.users == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	sessionID := auth.SessionFromContext(ctx)
	if req.Active {
		if err := h.users.TouchSession(ctx, sessionID); err != nil {
			respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
			return
		}
	}
	if auth.RoleFromContext(ctx) != auth.RoleViewer {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// 조회 전용(상황판)은 계속 띄워 두므로 만료를 뒤로 미루고 토큰을 새로 줌
	ttl := auth.TokenTTL(auth.RoleViewer)
	if err := h.users.ExtendSession(ctx, sessionID, time.Now().Add(ttl)); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	token, err := auth.GenerateToken(auth.UsernameFromContext(ctx), sessionID, h.jwtSecret, ttl)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	writeJSON(w, loginResponse{Token: token})
}

// Me GET /api/auth/me — 화면이 역할에 따라 메뉴·버튼을 숨기는 데 사용
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"username":   auth.UsernameFromContext(r.Context()),
		"role":       auth.RoleFromContext(r.Context()),
		"multi_user": h.users != nil,
	})
}

// ChangePassword POST /api/auth/password — 본인 비밀번호 변경
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if h.users == nil {
		http.Error(w, "환경변수로 설정된 계정은 화면에서 비밀번호를 바꿀 수 없습니다", http.StatusBadRequest)
		return
	}
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}
	user := auth.UsernameFromContext(r.Context())
	if _, reason := h.verify(r.Context(), user, req.Current); reason != "" {
		audit.LogFailure(r.Context(), "user.password_change", reason)
		http.Error(w, "현재 비밀번호가 올바르지 않습니다", http.StatusBadRequest)
		return
	}
	if msg := passwordPolicy(req.New); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	if err := h.users.SetPassword(r.Context(), user, string(hash), auth.SessionFromContext(r.Context())); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "계정을 찾을 수 없습니다", http.StatusNotFound)
			return
		}
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	audit.Log(r.Context(), "user.password_change", "user", user, nil)
	w.WriteHeader(http.StatusNoContent)
}

// passwordPolicy — 9자 이상 + 영문·숫자·특수문자 중 2종 이상 (공공기관 지침에서 흔한 기준)
func passwordPolicy(pw string) string {
	var letter, digit, special bool
	for _, c := range pw {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letter = true
		default:
			special = true
		}
	}
	kinds := 0
	for _, b := range []bool{letter, digit, special} {
		if b {
			kinds++
		}
	}
	if len([]rune(pw)) < 9 || kinds < 2 {
		return "비밀번호는 9자 이상, 영문·숫자·특수문자 중 2종류 이상을 섞어야 합니다"
	}
	return ""
}
