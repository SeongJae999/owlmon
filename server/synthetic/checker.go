// Package synthetic은 외부에서 URL을 주기적으로 호출해 가용성과 응답시간을 측정합니다.
package synthetic

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Monitor는 체크할 대상 정의입니다.
type Monitor struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Method          string `json:"method"`
	ExpectedStatus  int    `json:"expected_status"`
	ExpectedKeyword string `json:"expected_keyword"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	Enabled         bool   `json:"enabled"`
}

// Result는 단일 체크 결과입니다.
type Result struct {
	MonitorID      int64     `json:"monitor_id"`
	Success        bool      `json:"success"`
	StatusCode     int       `json:"status_code"`
	ResponseTimeMs int       `json:"response_time_ms"`
	Error          string    `json:"error,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
}

// MonitorLister는 등록된 모니터 목록을 제공합니다.
type MonitorLister interface {
	List(ctx context.Context) ([]Monitor, error)
}

// ResultSaver는 체크 결과를 저장합니다.
type ResultSaver interface {
	SaveResult(ctx context.Context, r Result) error
}

// Checker는 등록된 모든 모니터를 자체 주기로 체크합니다.
type Checker struct {
	lister MonitorLister
	saver  ResultSaver

	mu     sync.RWMutex
	latest map[int64]Result // monitor_id -> 최근 결과 (대시보드 빠른 조회용)
	fails  map[int64]int    // monitor_id -> 연속 실패 횟수
	stops  map[int64]chan struct{}

	// 알림 콜백 (3회 연속 실패/복구 시 호출)
	onDown     func(m Monitor, r Result, consecutive int)
	onRecovery func(m Monitor, r Result)
}

// NewChecker는 Checker를 생성합니다.
func NewChecker(lister MonitorLister, saver ResultSaver) *Checker {
	return &Checker{
		lister: lister,
		saver:  saver,
		latest: make(map[int64]Result),
		fails:  make(map[int64]int),
		stops:  make(map[int64]chan struct{}),
	}
}

// SetAlertCallbacks는 다운/복구 알림 콜백을 설정합니다.
func (c *Checker) SetAlertCallbacks(onDown func(Monitor, Result, int), onRecovery func(Monitor, Result)) {
	c.onDown = onDown
	c.onRecovery = onRecovery
}

// Start는 모니터 목록을 5분마다 동기화하면서 각 모니터의 자체 주기로 체크 루프를 돌립니다.
func (c *Checker) Start(ctx context.Context) {
	c.syncMonitors(ctx)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				c.stopAll()
				return
			case <-ticker.C:
				c.syncMonitors(ctx)
			}
		}
	}()
}

// syncMonitors는 등록된 모니터와 실행 중인 루프를 동기화합니다.
func (c *Checker) syncMonitors(ctx context.Context) {
	monitors, err := c.lister.List(ctx)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	active := make(map[int64]bool)
	for _, m := range monitors {
		if !m.Enabled {
			continue
		}
		active[m.ID] = true
		if _, ok := c.stops[m.ID]; !ok {
			stop := make(chan struct{})
			c.stops[m.ID] = stop
			go c.runMonitor(m, stop)
		}
	}
	// 비활성/삭제된 모니터 루프 정리
	for id, stop := range c.stops {
		if !active[id] {
			close(stop)
			delete(c.stops, id)
			delete(c.latest, id)
			delete(c.fails, id)
		}
	}
}

// runMonitor는 단일 모니터의 체크 루프입니다.
func (c *Checker) runMonitor(m Monitor, stop chan struct{}) {
	interval := time.Duration(m.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	c.executeOnce(m)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.executeOnce(m)
		}
	}
}

// executeOnce는 1회 체크 후 결과 저장과 알림 처리를 수행합니다.
func (c *Checker) executeOnce(m Monitor) {
	r := Check(m)
	_ = c.saver.SaveResult(context.Background(), r)

	c.mu.Lock()
	c.latest[m.ID] = r
	prevFails := c.fails[m.ID]
	if r.Success {
		c.fails[m.ID] = 0
	} else {
		c.fails[m.ID] = prevFails + 1
	}
	currentFails := c.fails[m.ID]
	c.mu.Unlock()

	// 알림: 3회 연속 실패 시 다운, 실패 후 첫 성공 시 복구
	if !r.Success && currentFails == 3 && c.onDown != nil {
		c.onDown(m, r, currentFails)
	}
	if r.Success && prevFails >= 3 && c.onRecovery != nil {
		c.onRecovery(m, r)
	}
}

// stopAll은 모든 모니터 루프를 중단합니다.
func (c *Checker) stopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, stop := range c.stops {
		close(stop)
		delete(c.stops, id)
	}
}

// GetLatest는 모든 모니터의 최근 결과를 반환합니다.
func (c *Checker) GetLatest() map[int64]Result {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[int64]Result, len(c.latest))
	for k, v := range c.latest {
		out[k] = v
	}
	return out
}

// TriggerOne은 단일 모니터를 즉시 체크합니다.
func (c *Checker) TriggerOne(m Monitor) Result {
	c.executeOnce(m)
	c.mu.RLock()
	r := c.latest[m.ID]
	c.mu.RUnlock()
	return r
}

// Check는 단일 HTTP 요청을 수행하고 결과를 반환합니다.
func Check(m Monitor) Result {
	r := Result{
		MonitorID: m.ID,
		CheckedAt: time.Now(),
	}

	method := strings.ToUpper(m.Method)
	if method == "" {
		method = "GET"
	}
	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	req, err := http.NewRequest(method, m.URL, nil)
	if err != nil {
		r.Error = fmt.Sprintf("요청 생성 실패: %v", err)
		return r
	}
	req.Header.Set("User-Agent", "OWLmon-Synthetic/1.0")

	start := time.Now()
	resp, err := client.Do(req)
	r.ResponseTimeMs = int(time.Since(start).Milliseconds())
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()

	r.StatusCode = resp.StatusCode
	expected := m.ExpectedStatus
	if expected == 0 {
		expected = 200
	}
	if resp.StatusCode != expected {
		r.Error = fmt.Sprintf("응답 코드 불일치 (기대: %d, 실제: %d)", expected, resp.StatusCode)
		return r
	}

	if m.ExpectedKeyword != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 최대 1MB
		if err != nil {
			r.Error = fmt.Sprintf("응답 읽기 실패: %v", err)
			return r
		}
		if !strings.Contains(string(body), m.ExpectedKeyword) {
			r.Error = fmt.Sprintf("응답 본문에 키워드 '%s' 없음", m.ExpectedKeyword)
			return r
		}
	}

	r.Success = true
	return r
}
