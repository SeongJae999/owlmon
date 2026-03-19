package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	consecutiveFailThreshold = 3 // 서비스 체크 연속 실패 횟수
)

// HistorySaver는 알림 히스토리를 저장하는 인터페이스입니다.
type HistorySaver interface {
	Save(ctx context.Context, host, category, severity, subject, body string) error
}

// Checker는 Prometheus를 주기적으로 조회하여 알림 조건을 평가합니다.
type Checker struct {
	prometheusURL string
	email         *EmailConfig
	state         *State
	configStore   ConfigStorer
	history       HistorySaver // nil이면 저장 안 함
}

func NewChecker(prometheusURL string, email *EmailConfig, configStore ConfigStorer, history HistorySaver) *Checker {
	return &Checker{
		prometheusURL: prometheusURL,
		email:         email,
		state:         NewState(),
		configStore:   configStore,
		history:       history,
	}
}

// Start는 백그라운드에서 주기적으로 알림 조건을 체크합니다.
func (c *Checker) Start(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			c.check()
		}
	}()
	log.Printf("알림 체커 시작 (주기: %v)", interval)
}

func (c *Checker) check() {
	cfg := c.configStore.Get()
	if !cfg.Enabled {
		return
	}
	// 수신자 목록을 동적으로 반영
	if len(cfg.Recipients) > 0 {
		c.email.To = cfg.Recipients
	}

	c.checkMetric("CPU", "max(system_cpu_usage_percent) by (host_name)", cfg.CPUThreshold, 0)
	c.checkMetric("메모리", "max(system_memory_usage_percent) by (host_name)", cfg.MemThreshold, 0)
	c.checkDisk(cfg.DiskWarn, cfg.DiskCrit)
	c.checkServerDown()
	c.checkServiceFailures()
}

// checkMetric은 단일 메트릭 임계값을 체크합니다.
func (c *Checker) checkMetric(name, promql string, critical, warning float64) {
	results, err := c.query(promql)
	if err != nil {
		return
	}
	for _, r := range results {
		host := r.metric["host_name"]
		val := r.value
		key := fmt.Sprintf("%s:%s:critical", name, host)
		if val >= critical {
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("🚨 %s %s 위험 (%.1f%%)", host, name, val)
				body := fmt.Sprintf("호스트: %s\n항목: %s 사용률\n현재 값: %.1f%%\n임계값: %.0f%%\n\n즉시 확인이 필요합니다.", host, name, val, critical)
				c.sendAlert(host, strings.ToLower(name), "critical", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(key) {
				subject := fmt.Sprintf("✅ %s %s 정상 복구 (%.1f%%)", host, name, val)
				body := fmt.Sprintf("호스트: %s\n항목: %s 사용률\n현재 값: %.1f%%\n\n정상 범위로 돌아왔습니다.", host, name, val)
				c.sendAlert(host, strings.ToLower(name), "info", subject, body)
			}
		}
	}
}

// checkDisk는 디스크를 경고/위험 두 단계로 체크합니다.
func (c *Checker) checkDisk(warn, crit float64) {
	results, err := c.query("max(system_disk_usage_percent) by (host_name, mountpoint)")
	if err != nil {
		return
	}
	for _, r := range results {
		host := r.metric["host_name"]
		mount := r.metric["mountpoint"]
		val := r.value
		critKey := fmt.Sprintf("디스크:%s:%s:critical", host, mount)
		warnKey := fmt.Sprintf("디스크:%s:%s:warning", host, mount)

		if val >= crit {
			if c.state.ShouldAlert(critKey) {
				subject := fmt.Sprintf("🚨 %s 디스크 위험 (%s %.1f%%)", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n디스크가 거의 꽉 찼습니다. 즉시 용량을 확보하세요.", host, mount, val)
				c.sendAlert(host, "disk", "critical", subject, body)
			}
		} else if val >= warn {
			c.state.ClearIfFiring(critKey)
			if c.state.ShouldAlert(warnKey) {
				subject := fmt.Sprintf("⚠️ %s 디스크 경고 (%s %.1f%%)", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n디스크 용량이 부족해지고 있습니다. 확인해 주세요.", host, mount, val)
				c.sendAlert(host, "disk", "warning", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(critKey) || c.state.ClearIfFiring(warnKey) {
				subject := fmt.Sprintf("✅ %s 디스크 정상 복구 (%s %.1f%%)", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n정상 범위로 돌아왔습니다.", host, mount, val)
				c.sendAlert(host, "disk", "info", subject, body)
			}
		}
	}
}

// checkServerDown은 최근 2분 내 데이터가 없는 호스트를 다운으로 판단합니다.
func (c *Checker) checkServerDown() {
	// 알려진 호스트 목록
	hosts, err := c.labelValues("host_name")
	if err != nil {
		return
	}
	for _, host := range hosts {
		results, err := c.query(fmt.Sprintf(`count_over_time(system_cpu_usage_percent{host_name="%s"}[2m])`, host))
		if err != nil {
			continue
		}
		key := fmt.Sprintf("down:%s", host)
		if len(results) == 0 {
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("🔴 %s 서버 연결 끊김", host)
				body := fmt.Sprintf("호스트: %s\n\n에이전트 연결이 끊겼습니다. 서버 상태를 확인하세요.", host)
				c.sendAlert(host, "down", "critical", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(key) {
				subject := fmt.Sprintf("✅ %s 서버 연결 복구", host)
				body := fmt.Sprintf("호스트: %s\n\n에이전트 연결이 복구되었습니다.", host)
				c.sendAlert(host, "down", "info", subject, body)
			}
		}
	}
}

// checkServiceFailures는 연속 실패 횟수가 임계값 이상인 서비스 체크를 알립니다.
func (c *Checker) checkServiceFailures() {
	results, err := c.query("service_check_status")
	if err != nil {
		return
	}
	for _, r := range results {
		name := r.metric["check_name"]
		host := r.metric["host_name"]
		target := r.metric["target"]
		key := fmt.Sprintf("svc:%s:%s", host, name)

		if r.value == 0 {
			count := c.state.RecordFailure(key)
			if count == consecutiveFailThreshold {
				subject := fmt.Sprintf("🚨 %s 서비스 장애 (%s)", host, name)
				body := fmt.Sprintf("호스트: %s\n서비스: %s\n대상: %s\n\n%d회 연속 응답 실패. 서비스 상태를 확인하세요.", host, name, target, count)
				c.sendAlert(host, "service", "critical", subject, body)
				c.state.ShouldAlert(key) // firing 상태 표시
			}
		} else {
			if c.state.ClearIfFiring(key) {
				subject := fmt.Sprintf("✅ %s 서비스 복구 (%s)", host, name)
				body := fmt.Sprintf("호스트: %s\n서비스: %s\n대상: %s\n\n서비스가 정상적으로 응답하고 있습니다.", host, name, target)
				c.sendAlert(host, "service", "info", subject, body)
			}
			c.state.ResetFailure(key)
		}
	}
}

func (c *Checker) sendAlert(host, category, severity, subject, body string) {
	if err := c.email.Send(subject, body); err != nil {
		log.Printf("알림 이메일 발송 실패: %v", err)
	} else {
		log.Printf("알림 발송: %s", subject)
	}
	if c.history != nil {
		if err := c.history.Save(context.Background(), host, category, severity, subject, body); err != nil {
			log.Printf("알림 히스토리 저장 실패: %v", err)
		}
	}
}

// --- Prometheus 조회 헬퍼 ---

type metricResult struct {
	metric map[string]string
	value  float64
}

func (c *Checker) query(promql string) ([]metricResult, error) {
	resp, err := http.Get(c.prometheusURL + "/api/v1/query?query=" + url.QueryEscape(promql))
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

	var out []metricResult
	for _, r := range result.Data.Result {
		valStr, _ := r.Value[1].(string)
		val, _ := strconv.ParseFloat(valStr, 64)
		out = append(out, metricResult{metric: r.Metric, value: val})
	}
	return out, nil
}

func (c *Checker) labelValues(label string) ([]string, error) {
	resp, err := http.Get(c.prometheusURL + "/api/v1/label/" + label + "/values")
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
