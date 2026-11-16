package snmp

import (
	"testing"
	"time"
)

// 동시 폴링 자리가 다 차 있으면 새 폴링은 기다려야 하고, 기다리는 장비는 다음 주기에 또 줄 서지 않아야 함
func TestPollWaitsForSlot(t *testing.T) {
	p := NewPoller()
	for range maxConcurrentPolls {
		p.slots <- struct{}{}
	}
	dev := Device{ID: 1} // 주소가 비어 있어 연결이 바로 실패 — 자리만 얻으면 곧 끝남
	done := make(chan struct{})
	go func() { p.Poll(dev); close(done) }()

	time.Sleep(100 * time.Millisecond)
	if p.Status(1) != nil {
		t.Fatal("자리가 없는데 폴링이 진행됨")
	}
	p.Poll(dev) // inflight라 바로 돌아와야 함 (막히면 테스트가 멈춤)

	<-p.slots
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("자리가 났는데 폴링이 끝나지 않음")
	}
	if p.Status(1) == nil {
		t.Fatal("폴링 결과가 저장되지 않음")
	}
}
