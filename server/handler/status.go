package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/prom"
)

// ActiveAlert는 현재 임계값을 초과 중인 알림 항목입니다.
type ActiveAlert struct {
	Host          string  `json:"host"`
	Category      string  `json:"category"` // cpu, memory, disk, down
	Severity      string  `json:"severity"` // warning, critical
	Value         float64 `json:"value"`
	Message       string  `json:"message"`
	Acked         bool    `json:"acked"`          // 확인됨 여부
	InMaintenance bool    `json:"in_maintenance"` // 유지보수 모드 여부
}

type StatusHandler struct {
	prometheusURL string
	configStore   alert.ConfigStorer
	checker       *alert.Checker
	dbPool        *pgxpool.Pool // 자가 진단용
}

func NewStatusHandler(prometheusURL string, configStore alert.ConfigStorer, checker *alert.Checker, dbPool *pgxpool.Pool) *StatusHandler {
	return &StatusHandler{
		prometheusURL: prometheusURL,
		configStore:   configStore,
		checker:       checker,
		dbPool:        dbPool,
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
			checks["postgres"] = "error: " + err.Error()
			status = "error"
		} else {
			checks["postgres"] = "ok"
		}
	} else {
		checks["postgres"] = "not_configured"
	}

	// 2. Prometheus 체크
	if _, err := h.labelValues("host_name"); err != nil {
		checks["prometheus"] = "error: " + err.Error()
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

// GetStatus는 현재 임계값 초과 중인 항목 목록을 반환합니다.
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	cfg := h.configStore.Get()
	var alerts []ActiveAlert

	// CPU
	if results, err := h.query("max(system_cpu_usage_percent) by (host_name)"); err == nil {
		for _, res := range results {
			if res.value >= cfg.CPUThreshold {
				alerts = append(alerts, ActiveAlert{
					Host:     res.metric["host_name"],
					Category: "cpu",
					Severity: "critical",
					Value:    res.value,
					Message:  fmt.Sprintf("CPU %.1f%% (임계값 %.0f%%)", res.value, cfg.CPUThreshold),
				})
			}
		}
	}

	// 메모리
	if results, err := h.query("max(system_memory_usage_percent) by (host_name)"); err == nil {
		for _, res := range results {
			if res.value >= cfg.MemThreshold {
				alerts = append(alerts, ActiveAlert{
					Host:     res.metric["host_name"],
					Category: "memory",
					Severity: "critical",
					Value:    res.value,
					Message:  fmt.Sprintf("메모리 %.1f%% (임계값 %.0f%%)", res.value, cfg.MemThreshold),
				})
			}
		}
	}

	// 디스크
	if results, err := h.query("max(system_disk_usage_percent) by (host_name, mountpoint)"); err == nil {
		for _, res := range results {
			mount := res.metric["mountpoint"]
			if res.value >= cfg.DiskCrit {
				alerts = append(alerts, ActiveAlert{
					Host:     res.metric["host_name"],
					Category: "disk",
					Severity: "critical",
					Value:    res.value,
					Message:  fmt.Sprintf("디스크 %s %.1f%% (임계값 %.0f%%)", mount, res.value, cfg.DiskCrit),
				})
			} else if res.value >= cfg.DiskWarn {
				alerts = append(alerts, ActiveAlert{
					Host:     res.metric["host_name"],
					Category: "disk",
					Severity: "warning",
					Value:    res.value,
					Message:  fmt.Sprintf("디스크 %s %.1f%% (경고 %.0f%%)", mount, res.value, cfg.DiskWarn),
				})
			}
		}
	}

	// 서버 다운 (2분 내 데이터 없음)
	if hosts, err := h.labelValues("host_name"); err == nil {
		for _, host := range hosts {
			results, err := h.query(fmt.Sprintf(`count_over_time(system_cpu_usage_percent{host_name="%s"}[2m])`, host))
			if err == nil && len(results) == 0 {
				alerts = append(alerts, ActiveAlert{
					Host:     host,
					Category: "down",
					Severity: "critical",
					Value:    0,
					Message:  "에이전트 연결 끊김",
				})
			}
		}
	}

	if alerts == nil {
		alerts = []ActiveAlert{}
	}
	// acked / in_maintenance 주석 추가
	if h.checker != nil {
		for i := range alerts {
			alerts[i].Acked = h.checker.IsAcked(alerts[i].Host, alerts[i].Category, alerts[i].Severity)
			alerts[i].InMaintenance = h.checker.IsInMaintenance(alerts[i].Host)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := make(map[string]float64)
	for _, host := range hosts {
		results, err := h.query(fmt.Sprintf(
			`count_over_time(system_cpu_usage_percent{host_name="%s"}[%s])`,
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
	resp, err := http.Get(h.prometheusURL + "/api/v1/query?query=" + url.QueryEscape(promql))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]interface{}    `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var out []promResult
	for _, r := range result.Data.Result {
		valStr, _ := r.Value[1].(string)
		val, _ := strconv.ParseFloat(valStr, 64)
		out = append(out, promResult{metric: r.Metric, value: val})
	}
	return out, nil
}

func (h *StatusHandler) labelValues(label string) ([]string, error) {
	// host_name은 공통 헬퍼로 — 같은 패턴이 checker.go, reporter.go에도 있어서 공통화함
	if label == "host_name" {
		hosts, err := prom.ActiveHosts(h.prometheusURL)
		if err == nil {
			return hosts, nil
		}
		// instant query 실패 시 기본 labelValues로 폴백
	}

	resp, err := http.Get(h.prometheusURL + "/api/v1/label/" + label + "/values")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
