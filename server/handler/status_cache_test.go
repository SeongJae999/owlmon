package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seongJae/owlmon/server/alert"
)

func TestCachedSharesInFlightAndSkipsErrors(t *testing.T) {
	var c cached[int]
	var calls atomic.Int32
	release := make(chan struct{})
	fetch := func() (int, error) {
		calls.Add(1)
		<-release
		return 7, nil
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := c.get(time.Minute, fetch); v != 7 || err != nil {
				t.Errorf("v=%d err=%v", v, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("동시 요청은 한 번만 조회해야 함: %d", n)
	}
	if v, _ := c.get(time.Minute, fetch); v != 7 || calls.Load() != 1 {
		t.Fatal("만료 전엔 저장값")
	}

	var e cached[int]
	fail := func() (int, error) { calls.Add(1); return 0, errors.New("x") }
	e.get(time.Minute, fail)
	e.get(time.Minute, fail)
	if n := calls.Load(); n != 3 {
		t.Fatalf("실패는 저장하지 않아야 함: %d", n)
	}
}

type countingProm struct {
	mu    sync.Mutex
	resp  map[string]string // 쿼리 종류 → 응답
	calls map[string]int
}

func (p *countingProm) kind(q string) string {
	switch {
	case strings.Contains(q, "count_over_time"):
		return "uptime"
	case strings.Contains(q, "[7d:5m]"):
		return "7d"
	case strings.Contains(q, "timestamp("):
		return "now"
	case strings.Contains(q, "system_cpu_usage_percent"):
		return "cpu"
	}
	return "other"
}

func (p *countingProm) set(kind, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resp[kind] = body
}

func (p *countingProm) count(kind string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[kind]
}

func newCountingProm(t *testing.T) (*countingProm, *StatusHandler) {
	p := &countingProm{resp: map[string]string{}, calls: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := p.kind(r.FormValue("query"))
		p.mu.Lock()
		p.calls[k]++
		body, ok := p.resp[k]
		p.mu.Unlock()
		if !ok {
			body = promVector()
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return p, NewStatusHandler(srv.URL, chatConfig{}, nil, nil)
}

func expire[T any](c *cached[T]) {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

func TestSnapshotRuns7dQueryOnlyOccasionally(t *testing.T) {
	now := float64(time.Now().Unix())
	p, h := newCountingProm(t)
	p.set("7d", promVector(row("web-01", now-120), row("old-01", now-600)))
	p.set("now", promVector(row("web-01", now-5)))

	s := h.snapshot()
	h.snapshot()
	if p.count("7d") != 1 || p.count("now") != 1 {
		t.Fatalf("10초 안 두 번째 요청은 Prometheus를 다시 부르면 안 됨: 7d=%d now=%d", p.count("7d"), p.count("now"))
	}
	if !s.Down["old-01"] || s.Down["web-01"] {
		t.Fatalf("down=%v", s.Down)
	}

	// 7일 조회 없이 최신 시각만으로 갱신 — 최신 쿼리에서 빠진 old-01은 메모리의 마지막 시각으로 남는다
	p.set("now", promVector(row("web-01", now-1), row("new-01", now-2)))
	expire(&h.promCache)
	s = h.snapshot()
	if p.count("7d") != 1 {
		t.Fatalf("10분 안에는 7일 조회를 다시 하면 안 됨: %d", p.count("7d"))
	}
	if got := s.Seen["old-01"]; got.Unix() != int64(now-600) || !s.Down["old-01"] {
		t.Errorf("꺼진 서버의 마지막 시각 유지: %v down=%v", got, s.Down["old-01"])
	}
	if s.Seen["web-01"].Unix() != int64(now-1) || s.Down["new-01"] {
		t.Errorf("최신 시각 반영: %v", s.Seen)
	}

	// 10분이 지나면 7일 조회로 새로 만든다 — 7일 안에 안 보인 서버는 빠진다
	p.set("7d", promVector(row("web-01", now-60)))
	h.seenFullAt = time.Now().Add(-seenFullEvery)
	expire(&h.promCache)
	s = h.snapshot()
	if p.count("7d") != 2 {
		t.Fatalf("10분 뒤 7일 조회: %d", p.count("7d"))
	}
	if _, ok := s.Seen["old-01"]; ok {
		t.Errorf("7일 조회에 없는 서버는 빠져야 함: %v", s.Seen)
	}

	// lookback보다 오래 갱신이 없었으면 그사이 마지막 시각을 놓쳤을 수 있어 7일 조회로 다시 맞춘다
	h.seenAt = time.Now().Add(-seenGap)
	expire(&h.promCache)
	h.snapshot()
	if p.count("7d") != 3 {
		t.Fatalf("갱신 공백 뒤 7일 조회: %d", p.count("7d"))
	}
}

func TestSnapshotAckIsNotCached(t *testing.T) {
	now := float64(time.Now().Unix())
	p, h := newCountingProm(t)
	p.set("7d", promVector(row("web-01", now)))
	p.set("now", promVector(row("web-01", now)))
	p.set("cpu", promVector(row("web-01", 99)))
	h.checker = alert.NewChecker("", nil, chatConfig{}, nil)

	if a := h.snapshot().Alerts; len(a) != 1 || a[0].Acked {
		t.Fatalf("alerts=%+v", a)
	}
	h.checker.Ack("web-01", "cpu", "critical", "kim")
	a := h.snapshot().Alerts
	if len(a) != 1 || !a[0].Acked || a[0].AckedBy != "kim" {
		t.Fatalf("확인은 캐시와 무관하게 바로 반영돼야 함: %+v", a)
	}
	if p.count("cpu") != 1 {
		t.Fatalf("Prometheus는 캐시: %d", p.count("cpu"))
	}
}

func TestUptimeOneQueryIncludesDownHosts(t *testing.T) {
	t.Setenv("OWLMON_COLLECT_INTERVAL_SEC", "15")
	now := float64(time.Now().Unix())
	p, h := newCountingProm(t)
	p.set("7d", promVector(row("web-01", now), row("old-01", now-3600)))
	p.set("now", promVector(row("web-01", now)))
	days := float64(time.Now().Day())
	expected := days * 24 * 240
	// 끝난 날마다 한 번 + 오늘 한 번 — 조회마다 같은 값을 주므로 날 수로 나눠 합이 기대값이 되게
	p.set("uptime", promVector(row("web-01", expected/2/days), row("big-01", expected*2/days)))

	for range 3 {
		rec := httptest.NewRecorder()
		h.GetUptime(rec, httptest.NewRequest("GET", "/api/uptime", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d", rec.Code)
		}
	}
	got, err := h.uptimeCache.get(uptimeCacheTTL, h.fetchUptime)
	if err != nil {
		t.Fatal(err)
	}
	if p.count("uptime") != int(days) {
		t.Fatalf("호스트 수와 무관하게 하루에 한 번, 5분 캐시: %d", p.count("uptime"))
	}
	if _, err := h.fetchUptime(); err != nil || p.count("uptime") != int(days)+1 {
		t.Fatalf("다시 셀 때는 오늘만: %d %v", p.count("uptime"), err)
	}
	if got["web-01"] != 50 || got["big-01"] != 100 {
		t.Errorf("가동률: %v", got)
	}
	if v, ok := got["old-01"]; !ok || v != 0 {
		t.Errorf("꺼진 서버도 포함: %v", got)
	}
}

// 에이전트 시계가 50초 늦어도 '수집 지연'으로 보이지 않게 — 샘플 시각이 바뀐 때를 서버 시계로 잼
func TestFreshnessUsesServerClock(t *testing.T) {
	now := float64(time.Now().Unix())
	p, h := newCountingProm(t)
	p.set("7d", promVector(row("web-01", now-50)))
	p.set("now", promVector(row("web-01", now-50))) // 늦은 시계로 찍힌 샘플
	if _, err := h.fetchProm(); err != nil {
		t.Fatal(err)
	}
	p.set("now", promVector(row("web-01", now-35))) // 15초 뒤 새 샘플
	if _, err := h.fetchProm(); err != nil {
		t.Fatal(err)
	}
	s := h.live.State("web-01", time.Now())
	if s.Age > 2*time.Second || s.Down {
		t.Fatalf("시계 차이가 나이·끊김에 섞임: %+v", s)
	}
	if s.Interval != 15*time.Second {
		t.Fatalf("수집 간격: %v", s.Interval)
	}
	// 시계가 늦은 서버도 화면 목록에서 끊김으로 보지 않음 (예전 기준은 '지금 − 샘플 시각 > 2분')
	if snap := h.snapshot(); snap.Down["web-01"] {
		t.Fatal("늦은 시계를 끊김으로 판정")
	}
}
