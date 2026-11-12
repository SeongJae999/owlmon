package synthetic

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// 실패 뒤 첫 성공마다 복구 콜백(확인 해제용)이 불리고, 다운 메일을 보낸 경우(3회 이상)에만 wasDown=true
func TestRecoveryCallback_WasDown(t *testing.T) {
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	m := Monitor{ID: 1, URL: srv.URL, Method: "GET", ExpectedStatus: 200, IntervalSeconds: 3600, TimeoutSeconds: 2, Enabled: true}
	c := NewChecker(&stubLister{}, nopSaver{})
	var downs int
	var recs []bool
	var starts int
	c.SetFailStartCallback(func(Monitor) { starts++ })
	c.SetAlertCallbacks(func(Monitor, Result, int) { downs++ }, func(_ Monitor, _ Result, wasDown bool) { recs = append(recs, wasDown) })

	ok = false
	c.executeOnce(m) // 1회 실패
	ok = true
	c.executeOnce(m) // 짧은 실패 뒤 복구 — 메일은 없지만 확인은 풀어야 함
	ok = false
	for i := 0; i < 3; i++ {
		c.executeOnce(m)
	}
	ok = true
	c.executeOnce(m)

	if starts != 2 {
		t.Fatalf("실패 시작 콜백 2회 기대, got %d", starts)
	}
	if downs != 1 {
		t.Fatalf("다운 콜백 1회 기대, got %d", downs)
	}
	if len(recs) != 2 || recs[0] || !recs[1] {
		t.Fatalf("복구 콜백 [false true] 기대, got %v", recs)
	}
}
