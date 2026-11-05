package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// 5회 실패 후엔 올바른 비밀번호도 429로 거부돼야 함.
func TestLogin_LockoutAfterFailures(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("right"), bcrypt.MinCost)
	h := NewAuthHandler("admin", string(hash), "secret", nil)
	login := func(pw string) int {
		rec := httptest.NewRecorder()
		h.Login(rec, httptest.NewRequest(http.MethodPost, "/api/login",
			strings.NewReader(`{"username":"admin","password":"`+pw+`"}`)))
		return rec.Code
	}
	if c := login("right"); c != http.StatusOK {
		t.Fatalf("정상 로그인 실패: %d", c)
	}
	for i := 0; i < maxLoginFailures; i++ {
		if c := login("wrong"); c != http.StatusUnauthorized {
			t.Fatalf("%d번째 실패 응답=%d", i+1, c)
		}
	}
	if c := login("right"); c != http.StatusTooManyRequests {
		t.Fatalf("잠금 중 로그인 응답=%d, 429 기대", c)
	}
}
