package synthetic

import (
	"context"
	"testing"
)

type stubLister struct{ ms []Monitor }

func (s *stubLister) List(context.Context) ([]Monitor, error) { return s.ms, nil }

type nopSaver struct{}

func (nopSaver) SaveResult(context.Context, Result) error { return nil }

// URL을 수정하면 기존 루프가 멈추고 새 설정으로 재시작돼야 함.
func TestSync_RestartsOnConfigChange(t *testing.T) {
	m := Monitor{ID: 1, URL: "http://127.0.0.1:1/a", Method: "GET", IntervalSeconds: 3600, TimeoutSeconds: 1, Enabled: true}
	l := &stubLister{ms: []Monitor{m}}
	c := NewChecker(l, nopSaver{})
	defer c.stopAll()

	c.Sync(context.Background())
	old := c.stops[1]

	m.URL = "http://127.0.0.1:1/b"
	l.ms = []Monitor{m}
	c.Sync(context.Background())

	select {
	case <-old:
	default:
		t.Fatal("설정 변경 후 기존 루프가 멈추지 않음")
	}
	if c.running[1].URL != m.URL {
		t.Fatalf("새 설정 미반영: %q", c.running[1].URL)
	}
}
