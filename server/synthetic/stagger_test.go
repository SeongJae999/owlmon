package synthetic

import (
	"testing"
	"time"
)

// 첫 검사 지연은 id로 정해지고(재시작해도 같음) 주기·1분 안이며, 연속 id는 서로 다른 시각에 흩어져야 함
func TestFirstDelaySpreads(t *testing.T) {
	seen := map[time.Duration]bool{}
	for id := int64(1); id <= 60; id++ {
		d := firstDelay(id, time.Hour)
		if d < 0 || d >= time.Minute {
			t.Fatalf("id %d: 지연 %v가 1분 범위를 벗어남", id, d)
		}
		if d != firstDelay(id, time.Hour) {
			t.Fatalf("id %d: 같은 id인데 지연이 다름", id)
		}
		seen[d] = true
	}
	if len(seen) != 60 {
		t.Fatalf("연속 id 60개가 %d개 시각에만 몰림", len(seen))
	}
	if d := firstDelay(25, 10*time.Second); d >= 10*time.Second {
		t.Fatalf("주기 10초인데 첫 지연 %v", d)
	}
}
