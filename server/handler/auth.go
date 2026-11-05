package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/auth"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	username     string
	passwordHash string // bcrypt 해시
	jwtSecret    string
	auditStore   *audit.Store

	// brute-force 방어 (ISMS-P: 연속 실패 시 잠금). 계정이 하나라 전역 카운터로 충분.
	mu          sync.Mutex
	failures    int
	lockedUntil time.Time
}

const (
	maxLoginFailures = 5
	loginLockout     = 10 * time.Minute
)

// lockRemaining — 잠금 중이면 남은 시간, 아니면 0.
func (h *AuthHandler) lockRemaining() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Until(h.lockedUntil)
}

// recordFailure — 실패 누적, 임계치 도달 시 잠금 시작. 잠금이 걸렸으면 true.
func (h *AuthHandler) recordFailure() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failures++
	if h.failures >= maxLoginFailures {
		h.failures = 0
		h.lockedUntil = time.Now().Add(loginLockout)
		return true
	}
	return false
}

func (h *AuthHandler) resetFailures() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failures = 0
}

func NewAuthHandler(username, passwordHash, jwtSecret string, auditStore *audit.Store) *AuthHandler {
	return &AuthHandler{
		username:     username,
		passwordHash: passwordHash,
		jwtSecret:    jwtSecret,
		auditStore:   auditStore,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// extractIP — proxy header 또는 RemoteAddr.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

// Login은 아이디/비밀번호를 확인하고 JWT를 발급합니다.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}

	ip := extractIP(r)
	ua := r.UserAgent()

	// 잠금 중에는 비밀번호가 맞아도 거부해야 무차별 대입이 무의미해짐
	if left := h.lockRemaining(); left > 0 {
		h.auditStore.Save(r.Context(), audit.Entry{
			Actor: req.Username, IP: ip, UserAgent: ua,
			Action: "auth.login", Result: "failure",
			Details: map[string]any{"reason": "locked"},
		})
		mins := int(left.Minutes()) + 1
		http.Error(w, fmt.Sprintf("로그인 %d회 실패로 잠겼습니다. %d분 후 다시 시도하세요", maxLoginFailures, mins), http.StatusTooManyRequests)
		return
	}

	// 아이디 확인
	if req.Username != h.username {
		h.auditStore.Save(r.Context(), audit.Entry{
			Actor:     req.Username,
			IP:        ip,
			UserAgent: ua,
			Action:    "auth.login",
			Result:    "failure",
			Details:   map[string]any{"reason": "unknown_user"},
		})
		h.failLogin(w, r, req.Username, ip, ua)
		return
	}

	// 비밀번호 확인 (bcrypt)
	if err := bcrypt.CompareHashAndPassword([]byte(h.passwordHash), []byte(req.Password)); err != nil {
		h.auditStore.Save(r.Context(), audit.Entry{
			Actor:     req.Username,
			IP:        ip,
			UserAgent: ua,
			Action:    "auth.login",
			Result:    "failure",
			Details:   map[string]any{"reason": "bad_password"},
		})
		h.failLogin(w, r, req.Username, ip, ua)
		return
	}

	h.resetFailures()
	token, err := auth.GenerateToken(req.Username, h.jwtSecret)
	if err != nil {
		http.Error(w, "토큰 생성 실패", http.StatusInternalServerError)
		return
	}

	h.auditStore.Save(r.Context(), audit.Entry{
		Actor:     req.Username,
		IP:        ip,
		UserAgent: ua,
		Action:    "auth.login",
		Result:    "success",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResponse{Token: token})
}

// failLogin — 실패 응답 + 누적. 이번 실패로 잠기면 감사 로그에 남김.
func (h *AuthHandler) failLogin(w http.ResponseWriter, r *http.Request, user, ip, ua string) {
	if h.recordFailure() {
		h.auditStore.Save(r.Context(), audit.Entry{
			Actor: user, IP: ip, UserAgent: ua,
			Action: "auth.lockout", Result: "failure",
			Details: map[string]any{"failures": maxLoginFailures, "minutes": int(loginLockout.Minutes())},
		})
	}
	http.Error(w, "아이디 또는 비밀번호가 올바르지 않습니다", http.StatusUnauthorized)
}
