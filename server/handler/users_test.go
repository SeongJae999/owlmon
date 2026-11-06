package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/auth"
	"golang.org/x/crypto/bcrypt"
)

type stubUsers struct {
	users map[string]struct {
		hash, role string
		disabled   bool
	}
	sessions map[string]bool
	touched  string
	extended string
}

func (s *stubUsers) ForLogin(_ context.Context, name string) (string, string, bool, error) {
	u, ok := s.users[name]
	if !ok {
		return "", "", false, pgx.ErrNoRows
	}
	return u.hash, u.role, u.disabled, nil
}
func (s *stubUsers) SetPassword(context.Context, string, string, string) error { return nil }
func (s *stubUsers) CreateSession(_ context.Context, id, _ string, _ time.Time) error {
	if s.sessions == nil {
		s.sessions = map[string]bool{}
	}
	s.sessions[id] = true
	return nil
}
func (s *stubUsers) TouchSession(_ context.Context, id string) error {
	s.touched = id
	return nil
}
func (s *stubUsers) ExtendSession(_ context.Context, id string, _ time.Time) error {
	s.extended = id
	return nil
}
func (s *stubUsers) DeleteSession(_ context.Context, id string) error {
	delete(s.sessions, id)
	return nil
}

func newMultiUserHandler(t *testing.T) *AuthHandler {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("right"), bcrypt.MinCost)
	st := &stubUsers{users: map[string]struct {
		hash, role string
		disabled   bool
	}{
		"admin":  {string(hash), auth.RoleAdmin, false},
		"viewer": {string(hash), auth.RoleViewer, false},
		"gone":   {string(hash), auth.RoleAdmin, true},
	}}
	return NewAuthHandler("envadmin", "unused", "secret", nil).WithUsers(st)
}

func doLogin(h *AuthHandler, user, pw string) int {
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"`+user+`","password":"`+pw+`"}`)))
	return rec.Code
}

func TestLogin_MultiUser(t *testing.T) {
	h := newMultiUserHandler(t)
	if c := doLogin(h, "viewer", "right"); c != http.StatusOK {
		t.Fatalf("등록된 계정 로그인 실패: %d", c)
	}
	if c := doLogin(h, "gone", "right"); c != http.StatusUnauthorized {
		t.Fatalf("사용 중지 계정은 거부돼야 함: %d", c)
	}
	if c := doLogin(h, "envadmin", "right"); c != http.StatusUnauthorized {
		t.Fatalf("다중 계정 모드에선 환경변수 계정으로 로그인되면 안 됨: %d", c)
	}
}

// 한 계정이 잠겨도 다른 계정은 로그인돼야 함 (단일 계정 시절엔 모두가 잠김)
func TestLogin_LockoutPerAccount(t *testing.T) {
	h := newMultiUserHandler(t)
	for i := 0; i < maxLoginFailures; i++ {
		doLogin(h, "admin", "wrong")
	}
	if c := doLogin(h, "admin", "right"); c != http.StatusTooManyRequests {
		t.Fatalf("admin 잠금 기대, got %d", c)
	}
	if c := doLogin(h, "viewer", "right"); c != http.StatusOK {
		t.Fatalf("다른 계정은 영향 없어야 함, got %d", c)
	}
}

func TestReadOnlyGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	g := auth.ReadOnlyGuard(ok)
	call := func(role, method, path string) int {
		mw := auth.JWTMiddleware("s", func(context.Context, string, string) (string, error) { return role, nil })
		tok, _ := auth.GenerateToken("u", "sid", "s", time.Hour)
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(g).ServeHTTP(rec, req)
		return rec.Code
	}
	cases := []struct {
		role, method, path string
		want               int
	}{
		{auth.RoleViewer, http.MethodGet, "/api/alert/config", 200},
		{auth.RoleViewer, http.MethodPost, "/api/alert/config", 403},
		{auth.RoleViewer, http.MethodPost, "/api/alert/ack", 200},     // 알림 확인은 허용
		{auth.RoleViewer, http.MethodPost, "/api/auth/password", 200}, // 내 비밀번호 변경 허용
		{auth.RoleViewer, http.MethodPost, "/api/auth/logout", 200},   // 로그아웃 허용
		{auth.RoleViewer, http.MethodDelete, "/api/snmp/devices/1", 403},
		{auth.RoleAdmin, http.MethodPost, "/api/alert/config", 200},
	}
	for _, c := range cases {
		if got := call(c.role, c.method, c.path); got != c.want {
			t.Errorf("%s %s %s = %d, want %d", c.role, c.method, c.path, got, c.want)
		}
	}
}

// 사용 중지된 계정은 이미 받은 토큰으로도 접근 못 해야 함
func TestJWTMiddleware_DisabledTokenRejected(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{auth.ErrSessionEnded, http.StatusUnauthorized},
		{auth.ErrIdle, http.StatusUnauthorized},
		{errors.New("db down"), http.StatusServiceUnavailable}, // DB 장애로 로그아웃되면 안 됨
	}
	for _, c := range cases {
		mw := auth.JWTMiddleware("s", func(context.Context, string, string) (string, error) { return "", c.err })
		tok, _ := auth.GenerateToken("u", "sid", "s", time.Hour)
		req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%v: got %d, want %d", c.err, rec.Code, c.want)
		}
	}
}

// 로그아웃한 토큰은 만료 전이어도 거부되고, 다른 세션은 유지돼야 함
func TestLogout_EndsOnlyThisSession(t *testing.T) {
	h := newMultiUserHandler(t)
	st := h.users.(*stubUsers)
	login := func() string {
		rec := httptest.NewRecorder()
		h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"username":"admin","password":"right"}`)))
		var res loginResponse
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		return res.Token
	}
	mw := auth.JWTMiddleware("secret", func(_ context.Context, _, sid string) (string, error) {
		if !st.sessions[sid] {
			return "", auth.ErrSessionEnded
		}
		return auth.RoleAdmin, nil
	})
	call := func(tok string, next http.HandlerFunc) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(next).ServeHTTP(rec, req)
		return rec.Code
	}
	noop := func(http.ResponseWriter, *http.Request) {}

	a, b := login(), login()
	if c := call(a, h.Logout); c != http.StatusNoContent {
		t.Fatalf("로그아웃 응답=%d", c)
	}
	if c := call(a, noop); c != http.StatusUnauthorized {
		t.Fatalf("로그아웃한 토큰 거부 기대, got %d", c)
	}
	if c := call(b, noop); c != http.StatusOK {
		t.Fatalf("다른 세션은 유지 기대, got %d", c)
	}
}

// 조회 전용만 토큰이 연장되고, 조작이 없으면 활동 시각을 갱신하면 안 됨
func TestHeartbeat(t *testing.T) {
	h := newMultiUserHandler(t)
	st := h.users.(*stubUsers)
	call := func(role, body string) *httptest.ResponseRecorder {
		st.touched, st.extended = "", ""
		mw := auth.JWTMiddleware("secret", func(context.Context, string, string) (string, error) { return role, nil })
		tok, _ := auth.GenerateToken("u", "sid", "secret", time.Hour)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/heartbeat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(h.Heartbeat)).ServeHTTP(rec, req)
		return rec
	}
	if rec := call(auth.RoleAdmin, `{"active":false}`); rec.Code != http.StatusNoContent || st.touched != "" || st.extended != "" {
		t.Fatalf("관리자 무조작: code=%d touched=%q extended=%q", rec.Code, st.touched, st.extended)
	}
	if rec := call(auth.RoleAdmin, `{"active":true}`); rec.Code != http.StatusNoContent || st.touched != "sid" || st.extended != "" {
		t.Fatalf("관리자 조작: code=%d touched=%q extended=%q", rec.Code, st.touched, st.extended)
	}
	rec := call(auth.RoleViewer, `{"active":false}`)
	var res loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil || res.Token == "" || st.extended != "sid" {
		t.Fatalf("조회 전용 연장: err=%v token=%q extended=%q", err, res.Token, st.extended)
	}
	if claims, err := auth.ValidateToken(res.Token, "secret"); err != nil || claims.ID != "sid" {
		t.Fatalf("연장 토큰은 같은 세션이어야 함: %v %+v", err, claims)
	}
}

// 조회 전용에게는 알림 수신자 이메일을 보여주지 않음
func TestAlertConfig_HidesRecipientsFromViewer(t *testing.T) {
	store := alert.NewConfigStore(filepath.Join(t.TempDir(), "alert.json"))
	cfg := store.Get()
	cfg.Recipients = []string{"ops@example.com"}
	if err := store.Set(cfg); err != nil {
		t.Fatal(err)
	}
	h := &AlertHandler{store: store}
	get := func(role string) string {
		mw := auth.JWTMiddleware("s", func(context.Context, string, string) (string, error) { return role, nil })
		tok, _ := auth.GenerateToken("u", "sid", "s", time.Hour)
		req := httptest.NewRequest(http.MethodGet, "/api/alert/config", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(h.GetConfig)).ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if strings.Contains(get(auth.RoleViewer), "ops@example.com") {
		t.Fatal("조회 전용 응답에 수신자가 있으면 안 됨")
	}
	if !strings.Contains(get(auth.RoleAdmin), "ops@example.com") {
		t.Fatal("관리자는 수신자를 봐야 함")
	}
}

func TestMaskKey(t *testing.T) {
	if got := maskKey("abcdef123456"); got != "abcd****" {
		t.Fatalf("got %q", got)
	}
	if got := maskKey("abc"); got != "****" {
		t.Fatalf("got %q", got)
	}
}

func TestPasswordPolicy(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short1!":     false, // 9자 미만
		"abcdefghij":  false, // 1종
		"abcdefgh12":  true,
		"한글비밀번호입니다12": true,
	} {
		if (passwordPolicy(pw) == "") != ok {
			t.Errorf("passwordPolicy(%q) ok=%v 기대", pw, ok)
		}
	}
}
