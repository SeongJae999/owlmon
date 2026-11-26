package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/snmp"
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

// DB 없는 단일 계정 모드에서 다른 이름(viewer·중지 계정)의 토큰이 관리자가 되면 안 됨
func TestJWTMiddleware_SingleAdminOnly(t *testing.T) {
	cases := []struct {
		user  string
		check auth.UserCheck
		want  int
	}{
		{"admin", auth.SingleAdmin("admin"), http.StatusOK},
		{"viewer1", auth.SingleAdmin("admin"), http.StatusUnauthorized},
		{"admin", nil, http.StatusUnauthorized},
	}
	for _, c := range cases {
		var role string
		tok, _ := auth.GenerateToken(c.user, "sid", "s", time.Hour)
		req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		auth.JWTMiddleware("s", c.check)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			role = auth.RoleFromContext(r.Context())
		})).ServeHTTP(rec, req)
		if rec.Code != c.want || (c.want == http.StatusOK && role != auth.RoleAdmin) {
			t.Errorf("%s nil=%v: got %d role=%q, want %d", c.user, c.check == nil, rec.Code, role, c.want)
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

// 남이 일부러 틀려도 다른 곳의 정상 사용자는 잠기지 않아야 함 (계정 잠금 DoS 방지)
func TestLogin_LockoutDoesNotBlockOtherIP(t *testing.T) {
	h := newMultiUserHandler(t)
	from := func(ip, user, pw string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"username":"`+user+`","password":"`+pw+`"}`))
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h.Login(rec, req)
		return rec.Code
	}
	for i := 0; i < maxLoginFailures; i++ {
		from("10.0.0.66", "admin", "wrong")
	}
	if c := from("10.0.0.66", "admin", "right"); c != http.StatusTooManyRequests {
		t.Fatalf("공격 IP는 잠겨야 함: %d", c)
	}
	if c := from("10.0.0.7", "admin", "right"); c != http.StatusOK {
		t.Fatalf("다른 IP의 관리자는 로그인돼야 함: %d", c)
	}
}

// 한 IP에서 계정을 바꿔 가며 대입하면 IP 단위로 잠겨야 함
func TestLogin_LockoutPerIP(t *testing.T) {
	h := newMultiUserHandler(t)
	from := func(ip, user, pw string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"username":"`+user+`","password":"`+pw+`"}`))
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h.Login(rec, req)
		return rec.Code
	}
	for i := 0; i < maxIPFailures; i++ {
		from("10.0.0.66", "user"+strconv.Itoa(i), "wrong")
	}
	if c := from("10.0.0.66", "viewer", "right"); c != http.StatusTooManyRequests {
		t.Fatalf("IP 한도 초과 시 다른 계정도 잠겨야 함: %d", c)
	}
	if c := from("10.0.0.7", "viewer", "right"); c != http.StatusOK {
		t.Fatalf("다른 IP는 영향 없어야 함: %d", c)
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

// 장비 이름은 gRPC 메타데이터(ASCII)·Prometheus 라벨로 그대로 쓰이므로 헤더 주입·한글이 들어가면 안 됨
func TestAgentNameRe(t *testing.T) {
	for _, ok := range []string{"web-01", "db.school.local", "SRV_2"} {
		if !agentNameRe.MatchString(ok) {
			t.Fatalf("허용해야 함: %q", ok)
		}
	}
	for _, bad := range []string{"", "-web", "a b", "웹서버", "x\r\nX-Owlmon-Host: y", strings.Repeat("a", 64)} {
		if agentNameRe.MatchString(bad) {
			t.Fatalf("거부해야 함: %q", bad)
		}
	}
}

// 비밀 값이 API 응답(JSON)으로 나가지 않아야 함 — 상태 API도 이 구조체를 그대로 내보냄
func TestSNMPDevice_HidesCommunity(t *testing.T) {
	b, err := json.Marshal(snmp.Device{Name: "sw1", Community: "s3cret"})
	if err != nil || strings.Contains(string(b), "s3cret") {
		t.Fatalf("community 노출: %s %v", b, err)
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

// os·arch에 ../ 를 넣어 저장 폴더 밖 파일(JWT 시크릿 등)을 읽지 못해야 함
func TestAgentBinary_RejectsPathTraversal(t *testing.T) {
	data := t.TempDir()
	store := filepath.Join(data, "agents")
	h := NewAgentUpdateHandler(store)
	if err := os.WriteFile(filepath.Join(data, ".owlmon_secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "owlmon-agent-linux-amd64"), []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"os=linux&arch=x/../../.owlmon_secret", "os=../.owlmon_secret&arch=amd64", "os=linux&arch=386"} {
		for _, fn := range []http.HandlerFunc{h.GetBinary, h.GetLatest} {
			rec := httptest.NewRecorder()
			fn(rec, httptest.NewRequest("GET", "/api/agent/binary?"+q, nil))
			if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "secret") {
				t.Fatalf("%s: 400 기대, got %d %q", q, rec.Code, rec.Body.String())
			}
		}
	}
	rec := httptest.NewRecorder()
	h.GetBinary(rec, httptest.NewRequest("GET", "/api/agent/binary?os=linux&arch=amd64", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "bin" {
		t.Fatalf("정상 요청: got %d %q", rec.Code, rec.Body.String())
	}
}

// 무인증 enroll — IP당 실패 한도를 넘으면 DB·감사 기록 없이 429, 다른 IP는 영향 없음
func TestEnroll_FailureLimitPerIP(t *testing.T) {
	h := NewAgentHandler(nil, nil, false, "", "")
	for i := 1; i < enrollMaxFailures; i++ {
		if h.enrollBlocked("10.0.0.1", true) {
			t.Fatalf("%d번째 실패에서 벌써 막힘", i)
		}
	}
	if !h.enrollBlocked("10.0.0.1", true) || !h.enrollBlocked("10.0.0.1", false) {
		t.Fatal("한도 도달 후 막혀야 함")
	}
	if h.enrollBlocked("10.0.0.2", false) {
		t.Fatal("다른 IP는 막히면 안 됨")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agent/enroll", strings.NewReader(`{"token":"x"}`))
	req.RemoteAddr = "10.0.0.1:5555"
	h.Enroll(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("429 기대, got %d", rec.Code)
	}
	h.fails["10.0.0.1"].since = time.Now().Add(-enrollFailWindow - time.Second)
	if h.enrollBlocked("10.0.0.1", false) {
		t.Fatal("창이 지나면 풀려야 함")
	}
}
