package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/seongJae/owlmon/server/alert"
)

// ActiveAlert는 현재 임계값을 초과 중인 알림 항목입니다.
type ActiveAlert struct {
	Host     string  `json:"host"`
	Category string  `json:"category"` // cpu, memory, disk, down
	Severity string  `json:"severity"` // warning, critical
	Value    float64 `json:"value"`
	Message  string  `json:"message"`
}

type StatusHandler struct {
	prometheusURL string
	configStore   alert.ConfigStorer
}

func NewStatusHandler(prometheusURL string, configStore alert.ConfigStorer) *StatusHandler {
	return &StatusHandler{prometheusURL: prometheusURL, configStore: configStore}
}

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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
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
