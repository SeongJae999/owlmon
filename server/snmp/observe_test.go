package snmp

import (
	"testing"
	"time"
)

var t0 = time.Now()

func at(i int) time.Time { return t0.Add(time.Duration(i) * 30 * time.Second) }

// 매 주기 새 샘플이 들어온 것처럼 에이전트 수집 값을 만듦
func samples(i int, vals map[string]bool) map[string]Sample {
	out := map[string]Sample{}
	for k, v := range vals {
		out[k] = Sample{Up: v, At: at(i)}
	}
	return out
}

func TestObserveTransitions(t *testing.T) {
	p := NewPoller()
	sw := Device{ID: 1, Name: "core-sw", IP: "10.0.0.1"}
	ap := Device{ID: 2, Name: "ap-1", IP: "10.0.0.2"}
	devs := []Device{sw, ap}
	down := map[string]bool{"core-sw": false, "ap-1": true}
	up := map[string]bool{"core-sw": true, "ap-1": true}
	obs := func(i int, v map[string]bool) []Event { return p.Observe(devs, samples(i, v), nil, at(i)) }

	// 1회 끊김은 알리지 않음, 2회 연속이면 장애 — 시작 시각은 첫 끊김
	if ev := obs(0, down); len(ev) != 0 {
		t.Fatalf("first miss: %+v", ev)
	}
	if d := p.DownSince(1); d == nil || !d.Equal(at(0)) {
		t.Fatalf("down since: %v", d)
	}
	ev := obs(1, down)
	if len(ev) != 1 || !ev[0].Down || ev[0].Device.Name != "core-sw" || !ev[0].Since.Equal(at(0)) {
		t.Fatalf("down: %+v", ev)
	}
	if ev := obs(2, down); len(ev) != 0 {
		t.Fatalf("still down repeats: %+v", ev)
	}
	// 복구도 연속 두 번 — 한 번 응답하고 다시 끊기면 복구 알림 없음
	if ev := obs(3, up); len(ev) != 0 {
		t.Fatalf("one up recovered: %+v", ev)
	}
	if ev := obs(4, down); len(ev) != 0 {
		t.Fatalf("flap re-alerted: %+v", ev)
	}
	obs(5, up)
	ev = obs(6, up)
	if len(ev) != 1 || ev[0].Down || !ev[0].Since.Equal(at(0)) || p.DownSince(1) != nil {
		t.Fatalf("recover: %+v", ev)
	}
	// 한 번 끊겼다 바로 돌아오면 아무 알림도 없음
	obs(7, down)
	if ev := obs(8, up); len(ev) != 0 {
		t.Fatalf("flap: %+v", ev)
	}
	// 서버 직접 폴링이 최근에 응답했으면 정상
	p.statuses[2] = &DeviceStatus{Device: ap, Up: true, CollectedAt: at(9)}
	if !p.Up(ap, map[string]bool{}, at(9).Add(time.Minute)) || p.Up(ap, map[string]bool{}, at(9).Add(3*time.Minute)) {
		t.Fatal("memory freshness")
	}
}

func TestObserveSameSampleCountsOnce(t *testing.T) {
	p := NewPoller()
	d := Device{ID: 1, Name: "sw"}
	// 수집 주기가 길어 같은 실패 샘플을 두 번 읽어도 한 번으로 셈
	old := map[string]Sample{"sw": {Up: false, At: at(0)}}
	p.Observe([]Device{d}, old, nil, at(0))
	if ev := p.Observe([]Device{d}, old, nil, at(1)); len(ev) != 0 {
		t.Fatalf("same sample counted twice: %+v", ev)
	}
	if ev := p.Observe([]Device{d}, samples(2, map[string]bool{"sw": false}), nil, at(2)); len(ev) != 1 {
		t.Fatalf("new sample: %+v", ev)
	}
}

func TestObserveNoEvidence(t *testing.T) {
	p := NewPoller()
	d := Device{ID: 1, Name: "sw"}
	// 첫 폴링 전·시계열 없음은 무응답으로 세지 않음
	for i := 0; i < 3; i++ {
		if ev := p.Observe([]Device{d}, nil, nil, at(i)); len(ev) != 0 || p.DownSince(1) != nil {
			t.Fatalf("no evidence counted: %+v", ev)
		}
	}
	// 서버만 폴링하는 장비는 서버 폴링 실패가 근거
	p.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(4)}
	p.Observe([]Device{d}, nil, nil, at(4))
	p.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(5)}
	if ev := p.Observe([]Device{d}, nil, nil, at(5)); len(ev) != 1 || !ev[0].Down {
		t.Fatalf("server-only down: %+v", ev)
	}

	// 최근 15분 안에 에이전트가 수집하던 장비는 시계열이 사라지면(수집 경로 끊김) 서버 폴링 실패만으로 장애로 보지 않음
	q := NewPoller()
	recent := map[string]bool{"sw": true}
	for i := 1; i < 4; i++ {
		q.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		if ev := q.Observe([]Device{d}, nil, recent, at(i)); len(ev) != 0 || !q.NoData(1) {
			t.Fatalf("agent path loss alerted or not marked no-data: %+v", ev)
		}
	}
	// 수집이 15분 넘게 없으면(recent에서 빠짐) 서버 폴링으로 판정 — 영영 보류되지 않음
	for i := 4; i < 6; i++ {
		q.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		if ev := q.Observe([]Device{d}, nil, nil, at(i)); i == 5 && (len(ev) != 1 || !ev[0].Down) {
			t.Fatalf("stale agent fallback: %+v", ev)
		}
	}
}

func TestObserveReachAndClock(t *testing.T) {
	d := Device{ID: 1, Name: "sw"}
	seen := map[string]bool{"sw": true}

	// 서버가 닿는 장비: 에이전트가 끊겨 시계열이 없어도 서버 폴링 실패가 근거
	p := NewPoller()
	p.statuses[1] = &DeviceStatus{Device: d, Up: true, CollectedAt: at(0)}
	p.Observe([]Device{d}, nil, seen, at(0))
	for i := 1; i <= 2; i++ {
		p.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		if ev := p.Observe([]Device{d}, nil, seen, at(i)); i == 2 && (len(ev) != 1 || !ev[0].Down) {
			t.Fatalf("reachable device down: %+v", ev)
		}
	}

	// 서버가 못 닿는 장비: 에이전트 샘플이 그대로면 서버 폴링 실패 시각이 흘러도 다시 세지 않음
	q := NewPoller()
	stale := map[string]Sample{"sw": {Up: false, At: at(0)}}
	for i := 0; i < 4; i++ {
		q.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		if ev := q.Observe([]Device{d}, stale, seen, at(i)); len(ev) != 0 {
			t.Fatalf("stale agent sample recounted: %+v", ev)
		}
	}

	// 에이전트 시계가 서버보다 2분 늦어도 복구가 늦어지지 않음
	r := NewPoller()
	skew := -4 // 30초 × 4
	agent := func(i int, up bool) map[string]Sample { return map[string]Sample{"sw": {Up: up, At: at(i + skew)}} }
	for i := 0; i < 2; i++ {
		r.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		r.Observe([]Device{d}, agent(i, false), nil, at(i))
	}
	r.Observe([]Device{d}, agent(2, true), nil, at(2))
	if ev := r.Observe([]Device{d}, agent(3, true), nil, at(3)); len(ev) != 1 || ev[0].Down {
		t.Fatalf("skewed recovery: %+v", ev)
	}
}

func TestObserveRemovedAndRenamed(t *testing.T) {
	p := NewPoller()
	a := Device{ID: 1, Name: "sw-a"}
	b := Device{ID: 2, Name: "sw-b"}
	off := map[string]bool{"sw-a": false, "sw-b": false, "sw-b2": false}
	p.Observe([]Device{a, b}, samples(0, off), nil, at(0))
	p.Observe([]Device{a, b}, samples(1, off), nil, at(1))

	// a 삭제, b 이름 변경 — 옛 이름 정리 이벤트, 새 이름은 처음부터 다시 셈
	b2 := Device{ID: 2, Name: "sw-b2"}
	ev := p.Observe([]Device{b2}, samples(2, off), nil, at(2))
	names := map[string]bool{}
	for _, e := range ev {
		if !e.Removed {
			t.Fatalf("unexpected: %+v", e)
		}
		names[e.Device.Name] = true
	}
	if len(ev) != 2 || !names["sw-a"] || !names["sw-b"] {
		t.Fatalf("removed events: %+v", ev)
	}
	if _, ok := p.watch[1]; ok {
		t.Fatal("deleted device kept")
	}
	ev = p.Observe([]Device{b2}, samples(3, off), nil, at(3))
	if len(ev) != 1 || !ev[0].Down || ev[0].Device.Name != "sw-b2" {
		t.Fatalf("renamed device down: %+v", ev)
	}
}
