package snmp

import "testing"

// 서버가 닿는 장비는 공유 에이전트 키로 보낸 up=1이 섞여도 서버 폴링 실패로 장애가 나고, 거짓 복구가 없어야 함
func TestObserveServerFailureBeatsAgentUp(t *testing.T) {
	p := NewPoller()
	d := Device{ID: 1, Name: "core-sw"}
	p.statuses[1] = &DeviceStatus{Device: d, Up: true, CollectedAt: at(0)}
	p.Observe([]Device{d}, nil, nil, at(0))

	fake := func(i int) map[string]Sample { return samples(i, map[string]bool{"core-sw": true}) }
	var downs, ups int
	for i := 1; i <= 6; i++ {
		p.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		for _, ev := range p.Observe([]Device{d}, fake(i), nil, at(i)) {
			if ev.Down {
				downs++
				if !ev.Conflict {
					t.Fatalf("충돌 표시 없음: %+v", ev)
				}
			} else {
				ups++
			}
		}
		if p.Up(d, map[string]bool{"core-sw": true}, at(i)) {
			t.Fatalf("tick %d: 화면이 정상으로 보임", i)
		}
	}
	if downs != 1 || ups != 0 {
		t.Fatalf("downs=%d ups=%d", downs, ups)
	}
}

// 재기동 뒤(메모리의 도달 기록 없음)에도 DB에 남긴 도달 기록이 있으면 위조 수집 기록으로 서버 폴링 실패가 무시되지 않아야 함
func TestObservePersistedReach(t *testing.T) {
	p := NewPoller()
	d := Device{ID: 1, Name: "core-sw", ServerReach: true}
	recent := map[string]bool{"core-sw": true}
	var ev []Event
	for i := 0; i < 2; i++ {
		p.statuses[1] = &DeviceStatus{Device: d, Up: false, CollectedAt: at(i)}
		ev = p.Observe([]Device{d}, nil, recent, at(i))
	}
	if len(ev) != 1 || !ev[0].Down {
		t.Fatalf("got %+v", ev)
	}
}

// 근거 없음(수집 끊김)이 15분 넘게 이어지면 한 번 알리고, 근거가 돌아오면 다시 셈
func TestObserveNoDataAlert(t *testing.T) {
	p := NewPoller()
	d := Device{ID: 1, Name: "edge-sw"}
	var got []Event
	for i := 0; i <= 40; i++ { // 30초 × 40 = 20분
		for _, ev := range p.Observe([]Device{d}, nil, nil, at(i)) {
			if ev.NoData {
				got = append(got, ev)
				if i != 30 {
					t.Fatalf("tick %d에 알림 (15분 = tick 30)", i)
				}
			}
		}
	}
	if len(got) != 1 || !got[0].Since.Equal(at(0)) {
		t.Fatalf("got %+v", got)
	}
	p.Observe([]Device{d}, samples(41, map[string]bool{"edge-sw": true}), nil, at(41))
	if w := p.watch[1]; w.noDataAlerted || !w.noDataSince.IsZero() {
		t.Fatalf("근거가 돌아왔는데 기록이 남음: %+v", w)
	}
}
