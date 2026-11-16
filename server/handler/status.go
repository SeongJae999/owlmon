package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/prom"
)

// ActiveAlert는 현재 임계값을 초과 중인 알림 항목입니다.
type ActiveAlert struct {
	Host          string     `json:"host"`
	Category      string     `json:"category"` // cpu, memory, disk, down
	Severity      string     `json:"severity"` // warning, critical
	Value         float64    `json:"value"`
	Message       string     `json:"message"`
	Acked         bool       `json:"acked"` // 확인됨 여부
	AckedBy       string     `json:"acked_by,omitempty"`
	AckedAt       *time.Time `json:"acked_at,omitempty"`
	Since         *time.Time `json:"since,omitempty"` // 이 문제가 처음 보인 시각
	InMaintenance bool       `json:"in_maintenance"`  // 유지보수 모드 여부
}

type StatusHandler struct {
	prometheusURL string
	configStore   alert.ConfigStorer
	checker       *alert.Checker
	dbPool        *pgxpool.Pool // 자가 진단용

	sinceMu sync.Mutex
	since   map[string]time.Time // 임계치 초과가 처음 보인 시각. 서버를 다시 켜면 그때부터 다시 잰다

	promCache   cached[promStatus]
	uptimeCache cached[map[string]float64]

	// fetchProm에서만 읽고 쓴다 — 캐시가 조회를 한 번에 하나만 돌린다
	lastSeen   map[string]time.Time
	seenFullAt time.Time // 7일 조회로 lastSeen을 새로 만든 시각
	seenAt     time.Time // 최신 시각 조회로 lastSeen을 갱신한 시각

	// fetchUptime에서만 읽고 쓴다. 지난 날의 호스트별 샘플 수 — 끝난 날은 바뀌지 않으므로 한 번만 셈
	dayCounts map[string]map[string]float64 // "2006-01-02" → host → 샘플 수
}

const (
	statusCacheTTL = 10 * time.Second
	uptimeCacheTTL = 5 * time.Minute
	seenFullEvery  = 10 * time.Minute
	// Prometheus lookback(5분)보다 오래 갱신이 끊기면 그사이 꺼진 서버의 마지막 시각을 놓치므로 7일 조회로 다시 맞춘다
	seenGap = 4 * time.Minute
)

var errFetchAborted = errors.New("조회가 중단됨")

// 화면 수만큼 같은 조회가 겹치지 않게 — 만료 전엔 저장값, 만료 뒤 동시 요청은 진행 중인 한 번의 결과를 같이 받는다.
// 실패는 저장하지 않아 다음 요청이 바로 다시 조회한다
type cached[T any] struct {
	mu   sync.Mutex
	at   time.Time
	val  T
	call *cacheCall[T]
}

type cacheCall[T any] struct {
	done chan struct{}
	val  T
	err  error
}

func (c *cached[T]) get(ttl time.Duration, fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	if !c.at.IsZero() && time.Since(c.at) < ttl {
		v := c.val
		c.mu.Unlock()
		return v, nil
	}
	if call := c.call; call != nil {
		c.mu.Unlock()
		<-call.done
		return call.val, call.err
	}
	call := &cacheCall[T]{done: make(chan struct{}), err: errFetchAborted} // fetch가 패닉해도 기다리던 요청이 풀리게
	c.call = call
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.call = nil
		if call.err == nil {
			c.val, c.at = call.val, time.Now()
		}
		c.mu.Unlock()
		close(call.done)
	}()
	call.val, call.err = fetch()
	return call.val, call.err
}

func NewStatusHandler(prometheusURL string, configStore alert.ConfigStorer, checker *alert.Checker, dbPool *pgxpool.Pool) *StatusHandler {
	return &StatusHandler{
		prometheusURL: prometheusURL,
		configStore:   configStore,
		checker:       checker,
		dbPool:        dbPool,
		since:         map[string]time.Time{},
	}
}

// HealthCheck는 시스템 자가 진단 결과를 반환합니다.
// GET /api/health
func (h *StatusHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	checks := make(map[string]string)

	// 1. PostgreSQL 체크
	if h.dbPool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.dbPool.Ping(ctx); err != nil {
			log.Printf("[health] postgres: %v", err) // 공개 엔드포인트라 내부 주소·원인은 로그에만
			checks["postgres"] = "error"
			status = "error"
		} else {
			checks["postgres"] = "ok"
		}
	} else {
		checks["postgres"] = "not_configured"
	}

	// 2. Prometheus 체크
	if _, err := h.labelValues("host_name"); err != nil {
		log.Printf("[health] prometheus: %v", err)
		checks["prometheus"] = "error"
		status = "error"
	} else {
		checks["prometheus"] = "ok"
	}

	// 3. 시스템 리소스
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	w.Header().Set("Content-Type", "application/json")
	if status != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"checks": checks,
		"server": map[string]interface{}{
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"num_cpu":    runtime.NumCPU(),
			"mem_alloc":  m.Alloc / 1024 / 1024,
			"uptime_sec": time.Since(serverStartTime).Seconds(),
		},
		"timestamp": time.Now(),
	})
}

var serverStartTime = time.Now()

// 경고 패널과 챗봇이 같은 판정을 쓰도록 한 곳에서 계산한다
type fleetSnapshot struct {
	CPU    map[string]float64
	Mem    map[string]float64
	Disk   map[string]map[string]float64
	Seen   map[string]time.Time
	Down   map[string]bool
	Maint  []string
	Alerts []ActiveAlert
	Failed bool
}

func (s fleetSnapshot) hosts() []string {
	out := make([]string, 0, len(s.Seen))
	for h := range s.Seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// Prometheus 결과만 캐시한다 — 임계치·확인·점검은 요청마다 최신으로 합쳐야 확인 버튼이 바로 반영된다
type promStatus struct {
	cpu, mem, disk []promResult
	seen           map[string]time.Time // nil이면 마지막 수신 시각을 몰라 끊김 판정을 건너뛴다
	failed         bool
}

func (h *StatusHandler) fetchProm() (promStatus, error) {
	now := time.Now()
	queries := []string{
		"max(system_cpu_usage_percent) by (host_name)",
		"max(system_memory_usage_percent) by (host_name)",
		"max(system_disk_usage_percent) by (host_name, mountpoint)",
		"max by (host_name) (timestamp(system_cpu_usage_percent))",
	}
	// 꺼진 지 5분이 넘으면 lookback에서 빠져 최신 시각 쿼리에 안 나온다. 7일 이력은 무거워 가끔만 보고 그사이엔 메모리에 이어 붙인다
	full := now.Sub(h.seenFullAt) >= seenFullEvery || now.Sub(h.seenAt) >= seenGap
	if full {
		queries = append(queries, "max by (host_name) (max_over_time(timestamp(system_cpu_usage_percent)[7d:5m]))")
	}
	results := make([][]promResult, len(queries))
	errs := make([]error, len(queries))
	var wg sync.WaitGroup
	for i, q := range queries {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			results[i], errs[i] = h.query(q)
		}(i, q)
	}
	wg.Wait()

	st := promStatus{cpu: results[0], mem: results[1], disk: results[2]}
	for _, err := range errs {
		if err != nil {
			log.Printf("[status] prometheus 조회 실패: %v", err)
			st.failed = true
		}
	}

	// 7일 쿼리는 5분 간격이라 살아 있는 서버도 늦게 보인다. 최신 시각 쿼리가 실패하면 오탐을 내지 않도록 판정을 건너뛴다
	if errs[3] != nil || (full && errs[4] != nil) {
		return st, nil
	}
	if full {
		h.lastSeen = map[string]time.Time{}
		for _, r := range results[4] {
			h.lastSeen[r.metric["host_name"]] = time.Unix(int64(r.value), 0)
		}
		h.seenFullAt = now
	}
	for _, r := range results[3] {
		t := time.Unix(int64(r.value), 0)
		host := r.metric["host_name"]
		if t.After(h.lastSeen[host]) {
			h.lastSeen[host] = t
		}
	}
	for host, t := range h.lastSeen {
		if now.Sub(t) > 7*24*time.Hour {
			delete(h.lastSeen, host)
		}
	}
	h.seenAt = now
	st.seen = maps.Clone(h.lastSeen)
	return st, nil
}

func (h *StatusHandler) snapshot() fleetSnapshot {
	st, _ := h.promCache.get(statusCacheTTL, h.fetchProm)
	snap := fleetSnapshot{
		CPU:    map[string]float64{},
		Mem:    map[string]float64{},
		Disk:   map[string]map[string]float64{},
		Seen:   map[string]time.Time{},
		Down:   map[string]bool{},
		Alerts: []ActiveAlert{},
		Failed: st.failed,
	}

	now := time.Now()
	for host, t := range st.seen {
		snap.Seen[host] = t
		if now.Sub(t) > 2*time.Minute {
			snap.Down[host] = true
		}
	}

	cfg := h.configStore.Get()
	for _, r := range st.cpu {
		host := r.metric["host_name"]
		snap.CPU[host] = r.value
		if r.value >= cfg.CPUThreshold && !snap.Down[host] {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "cpu",
				Severity: "critical",
				Value:    r.value,
				Message:  fmt.Sprintf("CPU %.1f%% (기준 %.0f%%)", r.value, cfg.CPUThreshold),
			})
		}
	}
	for _, r := range st.mem {
		host := r.metric["host_name"]
		snap.Mem[host] = r.value
		if r.value >= cfg.MemThreshold && !snap.Down[host] {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "memory",
				Severity: "critical",
				Value:    r.value,
				Message:  fmt.Sprintf("메모리 %.1f%% (기준 %.0f%%)", r.value, cfg.MemThreshold),
			})
		}
	}
	for _, r := range st.disk {
		host, mount := r.metric["host_name"], r.metric["mountpoint"]
		if snap.Disk[host] == nil {
			snap.Disk[host] = map[string]float64{}
		}
		snap.Disk[host][mount] = r.value
		if snap.Down[host] {
			continue
		}
		if r.value >= cfg.DiskCrit {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "disk",
				Severity: "critical",
				Value:    r.value,
				Message:  fmt.Sprintf("디스크 %s %.1f%% (기준 %.0f%%)", mount, r.value, cfg.DiskCrit),
			})
		} else if r.value >= cfg.DiskWarn {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "disk",
				Severity: "warning",
				Value:    r.value,
				Message:  fmt.Sprintf("디스크 %s %.1f%% (기준 %.0f%%)", mount, r.value, cfg.DiskWarn),
			})
		}
	}
	for _, host := range snap.hosts() {
		if snap.Down[host] {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "down",
				Severity: "critical",
				Message:  "에이전트 연결 끊김",
			})
		}
	}

	h.stampSince(snap, now)

	if h.checker != nil {
		snap.Maint = h.checker.MaintenanceHosts()
		for i := range snap.Alerts {
			a := &snap.Alerts[i]
			if info, ok := h.checker.AckInfo(a.Host, a.Category, a.Severity); ok {
				a.Acked, a.AckedBy, a.AckedAt = true, info.By, &info.At
			}
			a.InMaintenance = h.checker.IsInMaintenance(a.Host)
		}
	}
	return snap
}

func (h *StatusHandler) stampSince(snap fleetSnapshot, now time.Time) {
	h.sinceMu.Lock()
	defer h.sinceMu.Unlock()
	seen := map[string]bool{}
	for i := range snap.Alerts {
		a := &snap.Alerts[i]
		// 연결 끊김은 마지막으로 데이터가 들어온 시각이 곧 시작 시각
		if t, ok := snap.Seen[a.Host]; a.Category == "down" && ok {
			a.Since = &t
			continue
		}
		key := ackKey(a.Host, a.Category, a.Severity)
		seen[key] = true
		t, ok := h.since[key]
		if !ok {
			t = now
			h.since[key] = t
		}
		a.Since = &t
	}
	for key := range h.since {
		if !seen[key] {
			delete(h.since, key)
		}
	}
}

func ackKey(host, category, severity string) string {
	return host + "/" + category + "/" + severity
}

// GetStatus는 현재 임계값 초과 중인 항목 목록을 반환합니다.
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.snapshot().Alerts)
}

// GetUptime은 이번 달 호스트별 가동률(%)을 반환합니다.
// GET /api/uptime
func (h *StatusHandler) GetUptime(w http.ResponseWriter, r *http.Request) {
	result, err := h.uptimeCache.get(uptimeCacheTTL, h.fetchUptime)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *StatusHandler) fetchUptime() (map[string]float64, error) {
	daysElapsed := time.Now().Day() // 이번 달 경과 일수 (1~31)
	// 수집 주기 (env: OWLMON_COLLECT_INTERVAL_SEC, 기본 15초 — 에이전트 기본값과 일치)
	intervalSec := 15
	if v := os.Getenv("OWLMON_COLLECT_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			intervalSec = n
		}
	}
	expectedSamples := float64(daysElapsed * 24 * (3600 / intervalSec))

	// 한 달치를 한 번에 세면 수천 대에서 수억 샘플을 읽어 조회 시간 제한을 넘음 — 끝난 날은 하루씩 세어 기억하고 오늘만 다시 셈
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if h.dayCounts == nil {
		h.dayCounts = map[string]map[string]float64{}
	}
	sum := map[string]float64{}
	add := func(rs []promResult) {
		for _, r := range rs {
			sum[r.metric["host_name"]] += r.value
		}
	}
	keep := map[string]bool{}
	for d := 1; d < daysElapsed; d++ {
		end := dayStart.AddDate(0, 0, d-daysElapsed+1) // d일의 다음 날 0시
		key := end.AddDate(0, 0, -1).Format("2006-01-02")
		keep[key] = true
		counts, ok := h.dayCounts[key]
		if !ok {
			rs, err := h.queryAt(`sum by (host_name) (count_over_time(system_cpu_usage_percent[1d]))`, end)
			if err != nil {
				return nil, err
			}
			counts = map[string]float64{}
			for _, r := range rs {
				counts[r.metric["host_name"]] = r.value
			}
			h.dayCounts[key] = counts
		}
		for host, n := range counts {
			sum[host] += n
		}
	}
	for key := range h.dayCounts {
		if !keep[key] {
			delete(h.dayCounts, key) // 지난달 기록
		}
	}
	if secs := int(now.Sub(dayStart).Seconds()); secs > 0 {
		rs, err := h.query(fmt.Sprintf(`sum by (host_name) (count_over_time(system_cpu_usage_percent[%ds]))`, secs))
		if err != nil {
			return nil, err
		}
		add(rs)
	}

	result := map[string]float64{}
	// 화면 목록(7일 안에 본 서버)에 있는데 이번 달 샘플이 없으면 0%. 목록을 못 받으면 꺼진 서버가 빠진 결과를 5분 동안 기억하게 되므로 실패로 돌림
	st, err := h.promCache.get(statusCacheTTL, h.fetchProm)
	if err != nil || st.seen == nil {
		return nil, fmt.Errorf("서버 목록 조회 실패: %v", err)
	}
	for host := range st.seen {
		result[host] = 0
	}
	for host, n := range sum {
		result[host] = min(n/expectedSamples*100, 100)
	}
	return result, nil
}

type promResult struct {
	metric map[string]string
	value  float64
}

func (h *StatusHandler) query(promql string) ([]promResult, error) {
	return h.queryAt(promql, time.Time{})
}

func (h *StatusHandler) queryAt(promql string, at time.Time) ([]promResult, error) {
	// HTTP 호출/파싱은 prom 패키지 공용 클라이언트로 (타임아웃 포함)
	rs, err := prom.QueryAt(h.prometheusURL, promql, at)
	if err != nil {
		return nil, err
	}
	out := make([]promResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, promResult{metric: r.Metric, value: r.Value})
	}
	return out, nil
}

func (h *StatusHandler) labelValues(label string) ([]string, error) {
	// host_name의 stale 처리 포함 — prom.LabelValues가 ActiveHosts 우선 사용
	return prom.LabelValues(h.prometheusURL, label)
}
