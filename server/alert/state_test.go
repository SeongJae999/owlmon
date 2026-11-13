package alert

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 첫 알림은 즉시 발화한다.
func TestShouldAlert_FirstTime(t *testing.T) {
	s := NewState()
	if !s.ShouldAlert("host1/cpu") {
		t.Fatal("첫 알림은 발화해야 함")
	}
}

// 쿨다운(30분) 내 같은 키는 억제, 쿨다운 경과 후엔 재발화한다.
func TestShouldAlert_Cooldown(t *testing.T) {
	s := NewState()
	s.ShouldAlert("host1/cpu")

	if s.ShouldAlert("host1/cpu") {
		t.Fatal("쿨다운 내 재알림은 억제돼야 함")
	}

	// 쿨다운 경과 시뮬레이션 — 마지막 발송 시각을 과거로 되돌림
	s.mu.Lock()
	s.lastAlertAt["host1/cpu"] = time.Now().Add(-cooldown - time.Minute)
	s.mu.Unlock()

	if !s.ShouldAlert("host1/cpu") {
		t.Fatal("쿨다운 경과 후엔 재발화해야 함")
	}
}

// 쿨다운으로 억제돼도 firing 상태는 유지된다 — 회복 알림이 누락되지 않기 위한 핵심 동작.
func TestShouldAlert_SuppressedStillFiring(t *testing.T) {
	s := NewState()
	s.ShouldAlert("host1/mem")
	s.ShouldAlert("host1/mem") // 억제됨

	if !s.ClearIfFiring("host1/mem") {
		t.Fatal("억제된 알림도 firing 상태여야 회복 알림이 나간다")
	}
}

// 회복 알림은 발화 중이었을 때만 1회 나간다.
func TestClearIfFiring(t *testing.T) {
	s := NewState()

	if s.ClearIfFiring("host1/disk") {
		t.Fatal("발화한 적 없으면 회복 알림 없어야 함")
	}

	s.ShouldAlert("host1/disk")
	if !s.ClearIfFiring("host1/disk") {
		t.Fatal("발화 중이었으면 회복 알림이 나가야 함")
	}
	if s.ClearIfFiring("host1/disk") {
		t.Fatal("회복 처리 후 중복 회복 알림은 없어야 함")
	}
}

// 회복(Clear) 후 다시 임계 초과하면 쿨다운과 무관하게 즉시 재발화한다.
func TestClearResetsCooldown(t *testing.T) {
	s := NewState()
	s.ShouldAlert("host1/cpu")
	s.ClearIfFiring("host1/cpu")

	if !s.ShouldAlert("host1/cpu") {
		t.Fatal("회복 후 재발생한 알림은 즉시 발화해야 함")
	}
}

// 서비스 체크 연속 실패 카운트 증가/리셋.
func TestFailureCount(t *testing.T) {
	s := NewState()
	for want := 1; want <= 3; want++ {
		if got := s.RecordFailure("svc1"); got != want {
			t.Fatalf("연속 실패 %d회째인데 %d 반환", want, got)
		}
	}
	s.ResetFailure("svc1")
	if got := s.RecordFailure("svc1"); got != 1 {
		t.Fatalf("리셋 후 첫 실패는 1이어야 하는데 %d", got)
	}
}

// Ack 설정/조회/해제.
func TestAck(t *testing.T) {
	s := NewState()
	if s.IsAcked("h1", "cpu", "critical") {
		t.Fatal("ack 전엔 false여야 함")
	}
	s.Ack("h1", "cpu", "critical", "admin")
	if !s.IsAcked("h1", "cpu", "critical") {
		t.Fatal("ack 후엔 true여야 함")
	}
	if info, _ := s.AckInfo("h1", "cpu", "critical"); info.By != "admin" || info.At.IsZero() {
		t.Fatalf("확인한 사람과 시각이 남아야 함: %+v", info)
	}
	// 다른 severity는 별개 키
	if s.IsAcked("h1", "cpu", "warning") {
		t.Fatal("severity가 다르면 별개 ack여야 함")
	}
	s.ClearAck("h1", "cpu", "critical")
	if s.IsAcked("h1", "cpu", "critical") {
		t.Fatal("회복 시 ack가 해제돼야 함")
	}
}

// 유지보수 모드 설정/해제/목록.
func TestMaintenance(t *testing.T) {
	s := NewState()
	if s.IsInMaintenance("h1") {
		t.Fatal("기본값은 유지보수 아님")
	}
	s.SetMaintenance("h1", true)
	s.SetMaintenance("h2", true)
	if !s.IsInMaintenance("h1") {
		t.Fatal("설정 후엔 유지보수 모드여야 함")
	}
	if got := len(s.MaintenanceHosts()); got != 2 {
		t.Fatalf("유지보수 호스트 2대여야 하는데 %d대", got)
	}
	s.SetMaintenance("h1", false)
	if s.IsInMaintenance("h1") {
		t.Fatal("해제 후엔 유지보수 모드 아님")
	}
	if got := len(s.MaintenanceHosts()); got != 1 {
		t.Fatalf("해제 후 1대여야 하는데 %d대", got)
	}
}

func TestServerDownRecoveryClearsAckWithoutFiring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.FormValue("query")
		if strings.Contains(q, "count(system_cpu_usage_percent)") {
			fmt.Fprint(w, `{"status":"success","data":{"result":[{"metric":{"host_name":"web-01"},"value":[0,"1"]}]}}`)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"result":[{"metric":{},"value":[0,"8"]}]}}`)
	}))
	defer srv.Close()

	c := NewChecker(srv.URL, nil, nil, nil)
	c.state.Ack("web-01", "down", "critical", "admin")
	c.checkServerDown()
	if c.IsAcked("web-01", "down", "critical") {
		t.Fatal("서버가 복귀했는데 확인 상태가 남아 다음 장애 메일이 억제된다")
	}
}

// 확인 목록은 화면이 비서버 장비 상태를 그리는 근거 — 키 구성 요소와 확인자가 그대로 나와야 하고, 해제하면 빠져야 함
func TestAcks_ListAndClear(t *testing.T) {
	s := NewState()
	s.Ack("web-site", "synthetic", "critical", "kim")
	s.Ack("db-01", "dpm", "critical", "lee")
	got := map[string]AckInfo{}
	for _, a := range s.Acks() {
		got[a.Host] = a
	}
	if a := got["web-site"]; a.Category != "synthetic" || a.Severity != "critical" || a.By != "kim" || a.At.IsZero() {
		t.Fatalf("확인 정보 누락: %+v", a)
	}
	s.ClearAck("web-site", "synthetic", "critical")
	if len(s.Acks()) != 1 || s.IsAcked("web-site", "synthetic", "critical") {
		t.Fatal("해제한 확인이 남아 있음")
	}
}

func TestSSLAckResets(t *testing.T) {
	a := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	b := a.AddDate(0, 0, 45)
	now := a.AddDate(0, 0, -5)
	at := func(status string, n ...time.Time) sslSeen {
		p := sslSeen{}
		for _, x := range n {
			p = p.next(status, x, now)
		}
		return p
	}
	cases := []struct {
		name       string
		prev       sslSeen
		seen       bool
		status     string
		notAfter   time.Time
		crit, warn bool
	}{
		{"임박 유지", at("critical", a), true, "critical", a, false, false},
		{"임박→만료", at("critical", a), true, "expired", a, true, false},
		{"만료 유지", at("expired", a), true, "expired", a, false, false},
		{"갱신 후 주의 구간", at("critical", a), true, "warning", b, true, true},
		{"같은 인증서 주의→안내", at("warning", a), true, "notice", a, true, true},
		{"같은 인증서 주의 유지", at("warning", a), true, "warning", a, true, false},
		{"짧은 인증서 교체", at("critical", a), true, "critical", a.AddDate(0, 0, 3), true, true},
		{"추적 전 만료", sslSeen{}, false, "expired", a, true, false},
		{"추적 전 임박", sslSeen{}, false, "critical", a, false, false},
		{"분산 구성: 이미 본 다른 인증서 응답", at("critical", b, a), true, "critical", b, false, false},
		{"장기 인증서 뒤 단기 교체", at("critical", b.AddDate(1, 0, 0), a), true, "critical", a.AddDate(0, 0, 3), true, true},
	}
	for _, c := range cases {
		crit, warn := sslAckResets(c.prev, c.seen, c.status, c.notAfter)
		if crit != c.crit || warn != c.warn {
			t.Errorf("%s: got crit=%v warn=%v, want %v %v", c.name, crit, warn, c.crit, c.warn)
		}
	}
}

func TestSSLSeenNext(t *testing.T) {
	a := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	b := a.AddDate(0, 0, 1)
	now := a.AddDate(0, 0, -5)
	p := sslSeen{}.next("critical", b, now)
	p = p.next("critical", a, now.Add(6*time.Hour))
	if len(p.certs) != 2 {
		t.Fatalf("번갈아 응답하는 인증서 둘 다 기억 기대, got %v", p.certs)
	}
	p = p.next("critical", a, now.Add(8*24*time.Hour)) // 일주일 넘게 안 보인 인증서는 잊음
	if _, ok := p.certs[b.Unix()]; ok || len(p.certs) != 1 {
		t.Fatalf("오래 안 보인 인증서 폐기 기대, got %v", p.certs)
	}
}

func TestWithLocation(t *testing.T) {
	cases := []struct{ body, loc, want string }{
		{"호스트: web1\n현재 값: 96%", "2층 서버실 A랙 14U", "호스트: web1\n위치: 2층 서버실 A랙 14U\n현재 값: 96%"},
		{"이상 감지", "본관 1층", "이상 감지\n위치: 본관 1층\n"},
		{"호스트: web1\n본문", "", "호스트: web1\n본문"},
	}
	for _, c := range cases {
		if got := withLocation(c.body, c.loc); got != c.want {
			t.Errorf("withLocation(%q, %q) = %q, want %q", c.body, c.loc, got, c.want)
		}
	}
}
