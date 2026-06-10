package alert

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/anomaly"
	"github.com/seongJae/owlmon/server/prom"
	"github.com/seongJae/owlmon/server/ssl"
)

const (
	consecutiveFailThreshold = 3 // 서비스 체크 연속 실패 횟수
)

// HistorySaver는 알림 히스토리를 저장하는 인터페이스입니다.
type HistorySaver interface {
	Save(ctx context.Context, host, category, severity, subject, body string) error
}

// SSLDomainLister는 SSL 도메인 목록을 조회하는 인터페이스입니다.
type SSLDomainLister interface {
	ListEntries(ctx context.Context) ([]ssl.DomainEntry, error)
}

// Checker는 Prometheus를 주기적으로 조회하여 알림 조건을 평가합니다.
type Checker struct {
	prometheusURL  string
	email          *EmailConfig
	state          *State
	configStore    ConfigStorer
	history        HistorySaver // nil이면 저장 안 함
	Detector       *anomaly.Detector
	Predictor      *anomaly.Predictor
	sslDomainLister SSLDomainLister // nil이면 SSL 체크 비활성
	SSLChecker      *ssl.CertChecker
}

func NewChecker(prometheusURL string, email *EmailConfig, configStore ConfigStorer, history HistorySaver) *Checker {
	return &Checker{
		prometheusURL: prometheusURL,
		email:         email,
		state:         NewState(),
		configStore:   configStore,
		history:       history,
		Detector:      anomaly.NewDetector(),
		Predictor:     anomaly.NewPredictor(),
		SSLChecker:    ssl.NewCertChecker(),
	}
}

// SetSSLDomainLister는 SSL 도메인 목록 조회기를 설정합니다.
func (c *Checker) SetSSLDomainLister(lister SSLDomainLister) {
	c.sslDomainLister = lister
}

// StartSSLCheck는 SSL 인증서를 주기적으로 체크합니다.
func (c *Checker) StartSSLCheck(ctx context.Context, interval time.Duration) {
	if c.sslDomainLister == nil {
		return
	}
	go func() {
		// 시작 후 30초 뒤 첫 체크 (서버 초기화 대기) — 종료 시그널도 함께 대기
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		c.checkSSLCerts()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.checkSSLCerts()
			}
		}
	}()
	log.Printf("SSL 인증서 체커 시작 (주기: %v)", interval)
}

// checkSSLCerts는 등록된 도메인의 SSL 인증서를 체크합니다.
func (c *Checker) checkSSLCerts() {
	if c.sslDomainLister == nil {
		return
	}
	entries, err := c.sslDomainLister.ListEntries(context.Background())
	if err != nil {
		log.Printf("SSL 도메인 목록 조회 실패: %v", err)
		return
	}
	if len(entries) == 0 {
		return
	}

	results := c.SSLChecker.CheckAll(entries)

	cfg := c.configStore.Get()
	if !cfg.Enabled {
		return
	}
	if c.email != nil && len(cfg.Recipients) > 0 {
		c.email.To = cfg.Recipients
	}

	for _, info := range results {
		key := fmt.Sprintf("ssl:%s", info.Domain)
		switch info.Status {
		case "expired":
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("SSL 인증서 만료: %s", info.Domain)
				body := fmt.Sprintf("도메인: %s\n만료일: %s\n발급자: %s\n\nSSL 인증서가 만료되었습니다. 즉시 갱신하세요.",
					info.Domain, info.NotAfter.Format("2006-01-02"), info.Issuer)
				c.sendAlert(info.Domain, "ssl", "critical", subject, body)
			}
		case "critical":
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("SSL 인증서 %d일 후 만료: %s", info.DaysLeft, info.Domain)
				body := fmt.Sprintf("도메인: %s\n만료일: %s\n남은 일수: %d일\n발급자: %s\n\n인증서 갱신을 준비하세요.",
					info.Domain, info.NotAfter.Format("2006-01-02"), info.DaysLeft, info.Issuer)
				c.sendAlert(info.Domain, "ssl", "critical", subject, body)
			}
		case "warning":
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("SSL 인증서 %d일 후 만료: %s", info.DaysLeft, info.Domain)
				body := fmt.Sprintf("도메인: %s\n만료일: %s\n남은 일수: %d일\n발급자: %s\n\n인증서 갱신을 계획하세요.",
					info.Domain, info.NotAfter.Format("2006-01-02"), info.DaysLeft, info.Issuer)
				c.sendAlert(info.Domain, "ssl", "warning", subject, body)
			}
		case "notice":
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("SSL 인증서 %d일 남음 (갱신 준비): %s", info.DaysLeft, info.Domain)
				body := fmt.Sprintf("도메인: %s\n만료일: %s\n남은 일수: %d일\n발급자: %s\n\n갱신 결재/구매 절차를 시작하세요. (공공기관 lead time 고려)",
					info.Domain, info.NotAfter.Format("2006-01-02"), info.DaysLeft, info.Issuer)
				c.sendAlert(info.Domain, "ssl", "info", subject, body)
			}
		case "ok":
			c.state.ClearIfFiring(key)
		}
	}
}

// Start는 백그라운드에서 주기적으로 알림 조건을 체크합니다.
func (c *Checker) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.check()
			}
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
	if c.email != nil && len(cfg.Recipients) > 0 {
		c.email.To = cfg.Recipients
	}

	c.checkMetric("CPU", "max(system_cpu_usage_percent) by (host_name)", cfg.CPUThreshold, 0)
	c.checkMetric("메모리", "max(system_memory_usage_percent) by (host_name)", cfg.MemThreshold, 0)
	c.checkDisk(cfg.DiskWarn, cfg.DiskCrit)
	c.checkRapidGrowth()
	c.checkServerDown()
	c.checkServiceFailures()
	c.feedAnomalyDetector()
	c.feedDiskPredictor()
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
				subject := fmt.Sprintf("[심각] %s %s 위험 (%.1f%%)", host, name, val)
				body := fmt.Sprintf("호스트: %s\n항목: %s 사용률\n현재 값: %.1f%%\n임계값: %.0f%%\n\n즉시 확인이 필요합니다.", host, name, val, critical)
				c.sendAlert(host, strings.ToLower(name), "critical", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(key) {
				c.state.ClearAck(host, strings.ToLower(name), "critical")
				subject := fmt.Sprintf("[복구] %s %s 정상 복구 (%.1f%%)", host, name, val)
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
				subject := fmt.Sprintf("[심각] %s 디스크 위험 (%s %.1f%%)", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n디스크가 거의 꽉 찼습니다. 즉시 용량을 확보하세요.", host, mount, val)
				c.sendAlert(host, "disk", "critical", subject, body)
			}
		} else if val >= warn {
			c.state.ClearIfFiring(critKey)
			if c.state.ShouldAlert(warnKey) {
				subject := fmt.Sprintf("[주의] %s 디스크 경고 (%s %.1f%%)", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n디스크 용량이 부족해지고 있습니다. 확인해 주세요.", host, mount, val)
				c.sendAlert(host, "disk", "warning", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(critKey) || c.state.ClearIfFiring(warnKey) {
				c.state.ClearAck(host, "disk", "critical")
				c.state.ClearAck(host, "disk", "warning")
				subject := fmt.Sprintf("[복구] %s 디스크 정상 복구 (%s %.1f%%)", host, mount, val)
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
				c.state.ClearAck(host, "down", "critical")
				subject := fmt.Sprintf("[복구] %s 서버 연결 복구", host)
				body := fmt.Sprintf("호스트: %s\n\n에이전트 연결이 복구되었습니다.", host)
				c.sendAlert(host, "down", "info", subject, body)
			}
		}
	}
}

// checkRapidGrowth는 디스크/메모리가 단기간에 급변하는 경우를 알립니다.
// 단순 임계치(예: 디스크 85%)만 보면 "갑자기 50% → 86% 점프"를 못 감지 →
// 사후 알람만 가능. delta()로 1시간 변화량을 추적해 사전 감지.
func (c *Checker) checkRapidGrowth() {
	type growthCheck struct {
		name     string
		promql   string
		warnΔ    float64 // 임계 증가량 (%포인트)
		critΔ    float64
		category string
	}
	checks := []growthCheck{
		{"디스크", `delta(system_disk_usage_percent[1h])`, 5, 15, "rapid_disk"},
		{"메모리", `delta(system_memory_usage_percent[1h])`, 10, 25, "rapid_memory"},
	}
	for _, ck := range checks {
		results, err := c.query(ck.promql)
		if err != nil {
			continue
		}
		for _, r := range results {
			host := r.metric["host_name"]
			if host == "" {
				continue
			}
			if c.state.IsInMaintenance(host) {
				continue
			}
			growth := r.value
			var severity string
			switch {
			case growth >= ck.critΔ:
				severity = "critical"
			case growth >= ck.warnΔ:
				severity = "warning"
			default:
				continue
			}
			key := fmt.Sprintf("%s:%s", ck.category, host)
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("[예측] %s — %s 1시간 급증 (+%.1f%%p)", host, ck.name, growth)
				body := fmt.Sprintf("호스트: %s\n메트릭: %s\n1시간 전 대비 변화: +%.1f%%포인트\n\n갑작스러운 자원 사용 증가 — 빈번한 import/덤프/로그 폭증 등 의심.",
					host, ck.name, growth)
				c.sendAlert(host, ck.category, severity, subject, body)
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
				subject := fmt.Sprintf("[심각] %s 서비스 장애 (%s)", host, name)
				body := fmt.Sprintf("호스트: %s\n서비스: %s\n대상: %s\n\n%d회 연속 응답 실패. 서비스 상태를 확인하세요.", host, name, target, count)
				c.sendAlert(host, "service", "critical", subject, body)
				c.state.ShouldAlert(key) // firing 상태 표시
			}
		} else {
			if c.state.ClearIfFiring(key) {
				c.state.ClearAck(host, "service", "critical")
				subject := fmt.Sprintf("[복구] %s 서비스 복구 (%s)", host, name)
				body := fmt.Sprintf("호스트: %s\n서비스: %s\n대상: %s\n\n서비스가 정상적으로 응답하고 있습니다.", host, name, target)
				c.sendAlert(host, "service", "info", subject, body)
			}
			c.state.ResetFailure(key)
		}
	}
}

func (c *Checker) sendAlert(host, category, severity, subject, body string) {
	// 유지보수 모드: 이메일 및 히스토리 저장 모두 억제
	if c.state.IsInMaintenance(host) {
		log.Printf("유지보수 모드 — 알림 억제: %s", subject)
		return
	}
	// 확인된 알림: 이메일만 억제, 히스토리는 저장
	if severity != "info" && c.state.IsAcked(host, category, severity) {
		log.Printf("알림 확인됨 — 이메일 억제: %s", subject)
		if c.history != nil {
			_ = c.history.Save(context.Background(), host, category, severity, subject, body)
		}
		return
	}
	if c.email != nil {
		if err := c.email.Send(subject, body); err != nil {
			log.Printf("알림 이메일 발송 실패: %v", err)
		} else {
			log.Printf("알림 발송: %s", subject)
		}
	}
	if c.history != nil {
		if err := c.history.Save(context.Background(), host, category, severity, subject, body); err != nil {
			log.Printf("알림 히스토리 저장 실패: %v", err)
		}
	}
}

// SendAlert는 외부에서 알림을 발송할 수 있도록 sendAlert를 공개합니다.
// (synthetic, dpm 등 외부 모듈이 알림을 보낼 때 사용)
func (c *Checker) SendAlert(host, category, severity, subject, body string) {
	c.sendAlert(host, category, severity, subject, body)
}

// Ack는 알림을 확인 처리합니다.
func (c *Checker) Ack(host, category, severity string) {
	c.state.Ack(host, category, severity)
}

// IsAcked는 알림이 확인됐는지 반환합니다.
func (c *Checker) IsAcked(host, category, severity string) bool {
	return c.state.IsAcked(host, category, severity)
}

// SetMaintenance는 호스트의 유지보수 모드를 설정합니다.
func (c *Checker) SetMaintenance(host string, on bool) {
	c.state.SetMaintenance(host, on)
}

// IsInMaintenance는 호스트가 유지보수 모드인지 반환합니다.
func (c *Checker) IsInMaintenance(host string) bool {
	return c.state.IsInMaintenance(host)
}

// MaintenanceHosts는 유지보수 중인 호스트 목록을 반환합니다.
func (c *Checker) MaintenanceHosts() []string {
	return c.state.MaintenanceHosts()
}

// --- Prometheus 조회 헬퍼 ---

type metricResult struct {
	metric map[string]string
	value  float64
}

func (c *Checker) query(promql string) ([]metricResult, error) {
	// HTTP 호출/파싱은 prom 패키지 공용 클라이언트로 (타임아웃 포함)
	rs, err := prom.Query(c.prometheusURL, promql)
	if err != nil {
		return nil, err
	}
	out := make([]metricResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, metricResult{metric: r.Metric, value: r.Value})
	}
	return out, nil
}

// feedAnomalyDetector는 CPU/메모리 메트릭을 이상탐지 엔진에 주입합니다.
func (c *Checker) feedAnomalyDetector() {
	now := time.Now()

	// CPU
	if results, err := c.query("max(system_cpu_usage_percent) by (host_name)"); err == nil {
		for _, r := range results {
			host := r.metric["host_name"]
			if a := c.Detector.Feed(host, "cpu", r.value, now); a != nil {
				key := fmt.Sprintf("anomaly:cpu:%s", host)
				if a.Severity == "critical" && c.state.ShouldAlert(key) {
					subject := fmt.Sprintf("🔍 %s CPU 이상 감지 (Z=%.1f)", host, a.ZScore)
					c.sendAlert(host, "anomaly_cpu", a.Severity, subject, a.Message)
				}
			}
		}
	}

	// 메모리
	if results, err := c.query("max(system_memory_usage_percent) by (host_name)"); err == nil {
		for _, r := range results {
			host := r.metric["host_name"]
			if a := c.Detector.Feed(host, "memory", r.value, now); a != nil {
				key := fmt.Sprintf("anomaly:memory:%s", host)
				if a.Severity == "critical" && c.state.ShouldAlert(key) {
					subject := fmt.Sprintf("🔍 %s 메모리 이상 감지 (Z=%.1f)", host, a.ZScore)
					c.sendAlert(host, "anomaly_memory", a.Severity, subject, a.Message)
				}
			}
		}
	}
}

// feedDiskPredictor는 디스크 사용률을 예측 엔진에 주입합니다.
func (c *Checker) feedDiskPredictor() {
	now := time.Now()
	results, err := c.query("max(system_disk_usage_percent) by (host_name, mountpoint)")
	if err != nil {
		return
	}
	for _, r := range results {
		host := r.metric["host_name"]
		mount := r.metric["mountpoint"]
		pred := c.Predictor.Feed(host, mount, r.value, now)
		if pred == nil {
			continue
		}
		// 7일 이내 고갈 예상 + R² ≥ 0.5이면 알림
		if pred.DaysLeft >= 0 && pred.DaysLeft <= 7 && pred.R2 >= 0.5 {
			key := fmt.Sprintf("diskpred:%s:%s", host, mount)
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("[예측] %s 디스크 고갈 예측 (%s, %.0f일 후)", host, mount, pred.DaysLeft)
				c.sendAlert(host, "disk_prediction", "warning", subject, pred.Message)
			}
		}
	}
}

func (c *Checker) labelValues(label string) ([]string, error) {
	// host_name의 stale 처리 포함 — prom.LabelValues가 ActiveHosts 우선 사용
	return prom.LabelValues(c.prometheusURL, label)
}
