package alert

import (
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
	s.Ack("h1", "cpu", "critical")
	if !s.IsAcked("h1", "cpu", "critical") {
		t.Fatal("ack 후엔 true여야 함")
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
