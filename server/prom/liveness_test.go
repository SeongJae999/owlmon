package prom

import (
	"testing"
	"time"
)

func TestHostLiveness(t *testing.T) {
	var l HostLiveness
	t0 := time.Now()
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

	// 에이전트 시계가 90초 늦어도(샘플 시각이 과거) 바로 끊김으로 보지 않음
	l.Observe(map[string]float64{"slow": float64(t0.Unix() - 90), "ok": float64(t0.Unix())}, at(0))
	if s := l.State("slow", at(0)); s.Down || !s.Known {
		t.Fatalf("clock skew judged down: %+v", s)
	}
	// 15초마다 새 샘플 — 간격을 재고 끊김 아님
	l.Observe(map[string]float64{"slow": float64(t0.Unix() - 75), "ok": float64(t0.Unix() + 15)}, at(15))
	if s := l.State("slow", at(15)); s.Down || s.Interval != 15*time.Second {
		t.Fatalf("interval: %+v", s)
	}
	// 같은 샘플만 계속 오면(전송 멈춤) 45초 뒤 끊김
	l.Observe(map[string]float64{"slow": float64(t0.Unix() - 75), "ok": float64(t0.Unix() + 15)}, at(45))
	if s := l.State("ok", at(55)); s.Down {
		t.Fatalf("down too early: %+v", s)
	}
	if s := l.State("ok", at(61)); !s.Down {
		t.Fatalf("not down after 46s: %+v", s)
	}
	// 조회 결과에서 빠지면(5분 넘게 없음) 끊김, 기록도 지움
	l.Observe(map[string]float64{"slow": float64(t0.Unix() - 60)}, at(70))
	if s := l.State("ok", at(70)); !s.Down || s.Known {
		t.Fatalf("absent: %+v", s)
	}
	// 처음 볼 때 이미 3분 지난 샘플(서버 재시작 전부터 끊긴 서버)은 바로 끊김
	var m HostLiveness
	m.Observe(map[string]float64{"dead": float64(t0.Unix() - 180)}, t0)
	if s := m.State("dead", t0); !s.Down {
		t.Fatalf("already dead on first sight: %+v", s)
	}
	// 수집 주기 60초 에이전트는 기준이 3배(180초)
	if d := DownAfter(time.Minute); d != 3*time.Minute {
		t.Fatalf("down after: %v", d)
	}
}
