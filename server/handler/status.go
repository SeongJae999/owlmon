package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	InMaintenance bool       `json:"in_maintenance"` // 유지보수 모드 여부
}

type StatusHandler struct {
	prometheusURL string
	configStore   alert.ConfigStorer
	checker       *alert.Checker
	dbPool        *pgxpool.Pool // 자가 진단용

	sinceMu sync.Mutex
	since   map[string]time.Time // 임계치 초과가 처음 보인 시각. 서버를 다시 켜면 그때부터 다시 잰다
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

func (h *StatusHandler) snapshot() fleetSnapshot {
	queries := []string{
		"max(system_cpu_usage_percent) by (host_name)",
		"max(system_memory_usage_percent) by (host_name)",
		"max(system_disk_usage_percent) by (host_name, mountpoint)",
		"max by (host_name) (timestamp(system_cpu_usage_percent))",
		// 꺼진 지 5분이 넘으면 lookback에서 빠져 위 쿼리에 안 나온다. 7일 이력으로 찾아 계속 표시한다
		"max by (host_name) (max_over_time(timestamp(system_cpu_usage_percent)[7d:5m]))",
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

	snap := fleetSnapshot{
		CPU:    map[string]float64{},
		Mem:    map[string]float64{},
		Disk:   map[string]map[string]float64{},
		Seen:   map[string]time.Time{},
		Down:   map[string]bool{},
		Alerts: []ActiveAlert{},
	}
	for _, err := range errs {
		if err != nil {
			log.Printf("[status] prometheus 조회 실패: %v", err)
			snap.Failed = true
		}
	}

	// 7일 쿼리는 5분 간격이라 살아 있는 서버도 늦게 보인다. 최신 시각 쿼리가 실패하면 오탐을 내지 않도록 판정을 건너뛴다
	now := time.Now()
	if errs[3] == nil && errs[4] == nil {
		for _, rs := range results[3:5] {
			for _, r := range rs {
				t := time.Unix(int64(r.value), 0)
				if host := r.metric["host_name"]; t.After(snap.Seen[host]) {
					snap.Seen[host] = t
				}
			}
		}
		for host, t := range snap.Seen {
			if now.Sub(t) > 2*time.Minute {
				snap.Down[host] = true
			}
		}
	}

	cfg := h.configStore.Get()
	for _, r := range results[0] {
		host := r.metric["host_name"]
		snap.CPU[host] = r.value
		if r.value >= cfg.CPUThreshold && !snap.Down[host] {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "cpu",
				Severity: "critical",
				Value:    r.value,
				Message:  fmt.Sprintf("CPU %.1f%% (임계값 %.0f%%)", r.value, cfg.CPUThreshold),
			})
		}
	}
	for _, r := range results[1] {
		host := r.metric["host_name"]
		snap.Mem[host] = r.value
		if r.value >= cfg.MemThreshold && !snap.Down[host] {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "memory",
				Severity: "critical",
				Value:    r.value,
				Message:  fmt.Sprintf("메모리 %.1f%% (임계값 %.0f%%)", r.value, cfg.MemThreshold),
			})
		}
	}
	for _, r := range results[2] {
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
				Message:  fmt.Sprintf("디스크 %s %.1f%% (임계값 %.0f%%)", mount, r.value, cfg.DiskCrit),
			})
		} else if r.value >= cfg.DiskWarn {
			snap.Alerts = append(snap.Alerts, ActiveAlert{
				Host:     host,
				Category: "disk",
				Severity: "warning",
				Value:    r.value,
				Message:  fmt.Sprintf("디스크 %s %.1f%% (경고 %.0f%%)", mount, r.value, cfg.DiskWarn),
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
	now := time.Now()
	daysElapsed := now.Day() // 이번 달 경과 일수 (1~31)
	durationStr := fmt.Sprintf("%dd", daysElapsed)
	// 수집 주기 (env: OWLMON_COLLECT_INTERVAL_SEC, 기본 15초 — 에이전트 기본값과 일치)
	intervalSec := 15
	if v := os.Getenv("OWLMON_COLLECT_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			intervalSec = n
		}
	}
	samplesPerHour := 3600 / intervalSec
	expectedSamples := float64(daysElapsed * 24 * samplesPerHour)

	hosts, err := h.labelValues("host_name")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}

	result := make(map[string]float64)
	for _, host := range hosts {
		results, err := h.query(fmt.Sprintf(
			`count_over_time(system_cpu_usage_percent{host_name=%q}[%s])`,
			host, durationStr,
		))
		if err != nil || len(results) == 0 {
			result[host] = 0
			continue
		}
		pct := results[0].value / expectedSamples * 100
		if pct > 100 {
			pct = 100
		}
		result[host] = pct
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type promResult struct {
	metric map[string]string
	value  float64
}

func (h *StatusHandler) query(promql string) ([]promResult, error) {
	// HTTP 호출/파싱은 prom 패키지 공용 클라이언트로 (타임아웃 포함)
	rs, err := prom.Query(h.prometheusURL, promql)
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
