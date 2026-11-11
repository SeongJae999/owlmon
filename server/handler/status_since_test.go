package handler

import (
	"testing"
	"time"
)

func TestStampSince(t *testing.T) {
	h := NewStatusHandler("", nil, nil, nil)
	t0 := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	lastSeen := t0.Add(-7 * time.Minute)
	snap := func(alerts ...ActiveAlert) fleetSnapshot {
		return fleetSnapshot{Seen: map[string]time.Time{"db": lastSeen}, Alerts: alerts}
	}
	disk := ActiveAlert{Host: "web", Category: "disk", Severity: "critical"}
	down := ActiveAlert{Host: "db", Category: "down", Severity: "critical"}

	s := snap(disk, down)
	h.stampSince(s, t0)
	if !s.Alerts[0].Since.Equal(t0) || !s.Alerts[1].Since.Equal(lastSeen) {
		t.Fatalf("처음 본 시각과 마지막 수집 시각 기대: %v %v", s.Alerts[0].Since, s.Alerts[1].Since)
	}

	s = snap(disk)
	h.stampSince(s, t0.Add(10*time.Minute))
	if !s.Alerts[0].Since.Equal(t0) {
		t.Fatalf("계속 이어지는 문제는 처음 시각 유지: %v", s.Alerts[0].Since)
	}

	h.stampSince(snap(), t0.Add(20*time.Minute))
	s = snap(disk)
	h.stampSince(s, t0.Add(30*time.Minute))
	if !s.Alerts[0].Since.Equal(t0.Add(30 * time.Minute)) {
		t.Fatalf("풀렸다 다시 생기면 새로 잰다: %v", s.Alerts[0].Since)
	}
}
