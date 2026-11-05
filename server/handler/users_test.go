package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/auth"
	"golang.org/x/crypto/bcrypt"
)

type stubUsers struct {
	users map[string]struct {
		hash, role string
		disabled   bool
	}
}

func (s *stubUsers) ForLogin(_ context.Context, name string) (string, string, bool, error) {
	u, ok := s.users[name]
	if !ok {
		return "", "", false, pgx.ErrNoRows
	}
	return u.hash, u.role, u.disabled, nil
}
func (s *stubUsers) SetPassword(context.Context, string, string) error { return nil }

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
		mw := auth.JWTMiddleware("s", func(context.Context, string) (string, bool) { return role, true })
		tok, _ := auth.GenerateToken("u", "s")
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
	mw := auth.JWTMiddleware("s", func(context.Context, string) (string, bool) { return "", false })
	tok, _ := auth.GenerateToken("u", "s")
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("중지 계정 토큰 거부 기대, got %d", rec.Code)
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
