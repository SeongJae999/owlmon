package audit

import (
	"net/http/httptest"
	"testing"
	"time"
)

// 헤더는 nginx에서 온 요청일 때만 믿어야 함 — 8080 직접 접속으로 접속 기록 IP를 위조하지 못하게
func TestClientIP(t *testing.T) {
	lookupProxy = func(string) ([]string, error) { return []string{"172.18.0.5"}, nil }
	proxyAt = time.Time{}
	t.Setenv("OWLMON_TRUSTED_PROXY", "owlmon-web")

	cases := []struct {
		name, remote, realIP, xff, want string
	}{
		{"nginx 경유", "172.18.0.5:40000", "192.168.0.9", "", "192.168.0.9"},
		{"nginx 경유 XFF 위조 무시", "172.18.0.5:40000", "192.168.0.9", "1.2.3.4", "192.168.0.9"},
		{"직접 접속 헤더 위조 무시", "192.168.0.66:40000", "1.2.3.4", "1.2.3.4", "192.168.0.66"},
		{"헤더 없음", "192.168.0.66:40000", "", "", "192.168.0.66"},
		{"IPv6", "[fd00::1]:40000", "", "", "fd00::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}

	t.Setenv("OWLMON_TRUSTED_PROXY", "")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.18.0.5:40000"
	r.Header.Set("X-Real-IP", "1.2.3.4")
	if got := ClientIP(r); got != "172.18.0.5" {
		t.Errorf("설정이 없으면 헤더를 믿지 않아야 함: %s", got)
	}
}
