package alert

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
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
	prometheusURL   string
	email           *EmailConfig
	state           *State
	configStore     ConfigStorer
	history         HistorySaver // nil이면 저장 안 함
	Detector        *anomaly.Detector
	Predictor       *anomaly.Predictor
	sslDomainLister SSLDomainLister // nil이면 SSL 체크 비활성
	SSLChecker      *ssl.CertChecker
	sslMu           sync.Mutex
	sslPrev         map[string]sslSeen       // 도메인별 직전 인증서 상태 (만료 전환·교체 감지)
	locate          func(host string) string // nil이면 위치 줄 생략
	notifier        *Notifier
	escMu           sync.Mutex
	unacked         map[string]*pendingAlert // 단계 통보 대기 중인 위험 알림 (key: 호스트/항목/대상)
	watched         map[string]func(context.Context) (map[string]bool, error)
	downHosts       map[string]time.Time // 끊긴 시각, escMu로 보호
	cfgMu           sync.Mutex
	cfgVal          AlertConfig
	cfgAt           time.Time
	locMu           sync.Mutex
	locCache        map[string]cachedLoc
	sup             suppressor
	live            prom.HostLiveness // checkServerDown에서만 씀
}

type cachedLoc struct {
	loc string
	at  time.Time
}

// 장애 폭주 때 알림 1건마다 설정·위치를 DB에서 다시 읽지 않게 짧게 기억함. 설정은 저장 즉시 InvalidateConfig로 풂
const (
	configTTL   = 5 * time.Second
	locationTTL = time.Minute
)

type pendingAlert struct {
	host, category, target, subject, body string
	at                                    time.Time
	escalated                             bool
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
		sslPrev:       make(map[string]sslSeen),
		notifier:      NewNotifier(email),
		unacked:       make(map[string]*pendingAlert),
		watched:       make(map[string]func(context.Context) (map[string]bool, error)),
		downHosts:     make(map[string]time.Time),
		locCache:      make(map[string]cachedLoc),
	}
}

// SetWatched는 항목별로 지금 감시 중인 이름 목록을 알려 줌 — 장애 중에 끄거나 지우거나 이름을 바꾼 대상은 화면에서 확인할 수 없으니 2차 통보하지 않음.
// 시작 전에만 부름
func (c *Checker) SetWatched(category string, fn func(context.Context) (map[string]bool, error)) {
	c.watched[category] = fn
}

// Notifier는 설정 화면의 '시험 발송'이 같은 발송 코드를 쓰게 공개
func (c *Checker) Notifier() *Notifier { return c.notifier }

// IT 전담이 없는 기관은 메일 받은 사람이 장비 위치를 모름 — 자산 관리에 적은 위치를 본문에 붙인다
func (c *Checker) SetLocator(f func(host string) string) { c.locate = f }

func (c *Checker) location(host string) string {
	if c.locate == nil {
		return ""
	}
	c.locMu.Lock()
	defer c.locMu.Unlock()
	if e, ok := c.locCache[host]; ok && time.Since(e.at) < locationTTL {
		return e.loc
	}
	loc := c.locate(host)
	c.locCache[host] = cachedLoc{loc: loc, at: time.Now()}
	return loc
}

func (c *Checker) config() AlertConfig {
	c.cfgMu.Lock()
	defer c.cfgMu.Unlock()
	if c.cfgAt.IsZero() || time.Since(c.cfgAt) >= configTTL {
		c.cfgVal, c.cfgAt = c.configStore.Get(), time.Now()
	}
	return c.cfgVal
}

// InvalidateConfig는 설정 저장 직후 불러 다음 알림부터 새 설정을 쓰게 함
func (c *Checker) InvalidateConfig() {
	c.cfgMu.Lock()
	c.cfgAt = time.Time{}
	c.cfgMu.Unlock()
}

// 본문 첫 줄(호스트·도메인·모니터 등 대상)을 바로 뒤에 위치를 끼움
func withLocation(body, loc string) string {
	if loc == "" {
		return body
	}
	line := "위치: " + loc + "\n"
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		return body[:i+1] + line + body[i+1:]
	}
	return body + "\n" + line
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
	results := c.SSLChecker.CheckAll(entries)
	// 지운 도메인은 정리 분기에 닿지 않음 — 이번 목록에 없는 도메인의 단계 통보 대기를 버림
	current := make(map[string]bool, len(results))
	for _, info := range results {
		current[info.Domain] = true
	}
	c.dropUnacked(func(p *pendingAlert) bool { return p.category == "ssl" && !current[p.host] })
	if len(entries) == 0 {
		return
	}

	// 확인 해제는 알림이 꺼져 있어도 추적한다 — 꺼진 동안 눌린 확인이 켠 뒤의 만료 메일을 막지 않게
	for _, info := range results {
		// 접속 실패(error)는 인증서 상태를 모르는 것 — 직전 상태로 남기면 만료→오류→만료를 새 만료로 오인함
		if info.Status == "error" {
			continue
		}
		c.sslMu.Lock()
		prev, seen := c.sslPrev[info.Domain]
		c.sslPrev[info.Domain] = prev.next(info.Status, info.NotAfter, time.Now())
		c.sslMu.Unlock()
		clearCrit, clearWarn := sslAckResets(prev, seen, info.Status, info.NotAfter)
		if clearCrit {
			c.state.ClearAck(info.Domain, "ssl", "critical")
		}
		if clearWarn {
			c.state.ClearAck(info.Domain, "ssl", "warning")
		}
		// 갱신돼 위험 등급을 벗어나면 복구 알림이 없으므로 여기서 대기를 끝냄 — 갱신된 인증서로 2차 통보가 나가지 않게
		if info.Status != "expired" && info.Status != "critical" {
			c.dropUnacked(func(p *pendingAlert) bool { return p.host == info.Domain && p.category == "ssl" })
		}
	}

	if !c.config().Enabled {
		return
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

// 확인을 풀어야 하는 순간: 인증서 교체(짧은 기간 인증서는 늘 임박이라 상태 전환만으론 안 풀림),
// 만료로 넘어감(임박 때 확인한 것이 만료 메일까지 막지 않게 — 처음 보는 만료도 추적 전 확인일 수 있어 해제), 해당 등급을 벗어남
func sslAckResets(prev sslSeen, seen bool, status string, notAfter time.Time) (crit, warn bool) {
	_, known := prev.certs[notAfter.Unix()]
	replaced := seen && !known
	crit = replaced || (status == "expired" && (!seen || prev.status != "expired")) || (status != "expired" && status != "critical")
	warn = replaced || status == "ok" || status == "notice"
	return
}

// 교체 = 최근에 본 적 없는 만료일이 나타남. 분산 구성에서 인증서 여럿이 번갈아 응답해도 이미 본 것이라 교체로 보지 않는다.
// 한동안 안 보인 인증서는 잊어서, 같은 만료일이 아주 나중에 다시 보이는 드문 경우에만 해제가 한 번 더 일어남
const sslForget = 7 * 24 * time.Hour

type sslSeen struct {
	status string
	certs  map[int64]time.Time // 만료일(Unix) → 마지막으로 본 시각
}

func (p sslSeen) next(status string, notAfter, now time.Time) sslSeen {
	n := sslSeen{status: status, certs: map[int64]time.Time{notAfter.Unix(): now}}
	for k, at := range p.certs {
		if _, cur := n.certs[k]; !cur && now.Sub(at) < sslForget {
			n.certs[k] = at
		}
	}
	return n
}

// Start는 백그라운드에서 주기적으로 알림 조건을 체크합니다.
func (c *Checker) Start(ctx context.Context, interval time.Duration) {
	c.notifier.Start(ctx)
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
	cfg := c.config()
	if !cfg.Enabled {
		// 꺼 둔 동안의 장애가 다시 켤 때 한꺼번에 2차 통보되지 않게
		c.dropUnacked(func(*pendingAlert) bool { return true })
		return
	}
	c.escalate(cfg, time.Now())
	if !c.promHealthy() {
		return
	}

	// 임계 판정과 이상탐지·예측이 같은 조회 결과를 씀 — 실패하면 nil이라 양쪽 모두 이번 주기를 건너뜀
	cpu, _ := c.query("max(system_cpu_usage_percent) by (host_name)")
	mem, _ := c.query("max(system_memory_usage_percent) by (host_name)")
	disk, _ := c.query("max(system_disk_usage_percent) by (host_name, mountpoint)")
	c.checkMetric("CPU", "cpu", cpu, cfg.CPUThreshold, 0)
	c.checkMetric("메모리", "memory", mem, cfg.MemThreshold, 0)
	c.checkDisk(disk, cfg.DiskWarn, cfg.DiskCrit)
	c.checkRapidGrowth()
	c.checkServerDown()
	c.checkServiceFailures()
	c.feedAnomalyDetector(cpu, mem)
	c.feedDiskPredictor(disk)
}

// promHealthy는 Prometheus 응답 여부를 확인합니다.
// 조회 실패 시 모든 임계치 감시가 조용히 멈추므로, 연속 실패하면 알림으로 알립니다.
func (c *Checker) promHealthy() bool {
	const key = "prom:down"
	if _, err := c.query("up"); err != nil {
		n := c.state.RecordFailure(key)
		if n == 1 {
			log.Printf("Prometheus 조회 실패 — 알림 감시 일시 중단: %v", err)
		}
		if n == consecutiveFailThreshold && c.state.ShouldAlert(key) {
			subject := "[위험] Prometheus 응답 없음 — 알림 감시 중단"
			body := fmt.Sprintf("Prometheus 조회가 %d회 연속 실패했습니다.\n복구될 때까지 CPU·메모리·디스크·서버 다운 알림이 동작하지 않습니다.\n\n오류: %v", n, err)
			c.sendAlert("owlmon", "monitoring", "critical", subject, body)
		}
		return false
	}
	c.state.ResetFailure(key)
	if c.state.ClearIfFiring(key) {
		c.sendAlert("owlmon", "monitoring", "info", "[복구] Prometheus 응답 복구 — 알림 감시 재개", "Prometheus 조회가 정상화되어 알림 감시를 재개합니다.")
	}
	return true
}

// checkMetric은 단일 메트릭 임계값을 체크합니다.
// category는 /api/alert/status·ack와 같은 키여야 화면의 [확인]이 메일 억제에 반영됨
func (c *Checker) checkMetric(name, category string, results []metricResult, critical, warning float64) {
	for _, r := range results {
		host := r.metric["host_name"]
		val := r.value
		key := fmt.Sprintf("%s:%s:critical", name, host)
		if val >= critical {
			if c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("[위험] %s %s %.1f%%", host, name, val)
				body := fmt.Sprintf("호스트: %s\n항목: %s 사용률\n현재 값: %.1f%%\n임계값: %.0f%%\n\n즉시 확인이 필요합니다.", host, name, val, critical)
				c.sendAlert(host, category, "critical", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(key) {
				c.state.ClearAck(host, category, "critical")
				subject := fmt.Sprintf("[복구] %s %s 정상 복구 (%.1f%%)", host, name, val)
				body := fmt.Sprintf("호스트: %s\n항목: %s 사용률\n현재 값: %.1f%%\n\n정상 범위로 돌아왔습니다.", host, name, val)
				c.sendAlert(host, category, "info", subject, body)
			}
		}
	}
}

// checkDisk는 디스크를 경고/위험 두 단계로 체크합니다.
func (c *Checker) checkDisk(results []metricResult, warn, crit float64) {
	for _, r := range results {
		host := r.metric["host_name"]
		mount := r.metric["mountpoint"]
		val := r.value
		critKey := fmt.Sprintf("디스크:%s:%s:critical", host, mount)
		warnKey := fmt.Sprintf("디스크:%s:%s:warning", host, mount)

		if val >= crit {
			if c.state.ShouldAlert(critKey) {
				subject := fmt.Sprintf("[위험] %s 디스크 %s %.1f%%", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n디스크가 거의 꽉 찼습니다. 즉시 용량을 확보하세요.", host, mount, val)
				c.send(host, "disk", mount, "critical", subject, body)
			}
		} else if val >= warn {
			if c.state.ClearIfFiring(critKey) {
				c.dropUnacked(func(p *pendingAlert) bool { return p.host == host && p.category == "disk" && p.target == mount })
			}
			if c.state.ShouldAlert(warnKey) {
				subject := fmt.Sprintf("[주의] %s 디스크 %s %.1f%%", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n디스크 용량이 부족해지고 있습니다. 확인해 주세요.", host, mount, val)
				c.send(host, "disk", mount, "warning", subject, body)
			}
		} else {
			if c.state.ClearIfFiring(critKey) || c.state.ClearIfFiring(warnKey) {
				c.state.ClearAck(host, "disk", "critical")
				c.state.ClearAck(host, "disk", "warning")
				subject := fmt.Sprintf("[복구] %s 디스크 정상 복구 (%s %.1f%%)", host, mount, val)
				body := fmt.Sprintf("호스트: %s\n마운트: %s\n현재 사용률: %.1f%%\n\n정상 범위로 돌아왔습니다.", host, mount, val)
				c.send(host, "disk", mount, "info", subject, body)
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
	// 호스트마다 조회하면 3,000대에서 한 주기에 3,000번 — 최신 샘플 시각을 한 번에 받아 판정.
	// 기준은 화면과 같은 prom.HostLiveness(연속 3회 수집 없음, 서버 시계) — 예전 '2분간 샘플 없음'보다 빠르고 에이전트 시계 차이에 흔들리지 않음
	results, err := c.query(`max by (host_name) (timestamp(system_cpu_usage_percent))`)
	if err != nil {
		return
	}
	latest := make(map[string]float64, len(results))
	for _, r := range results {
		latest[r.metric["host_name"]] = r.value
	}
	now := time.Now()
	c.live.Observe(latest, now)
	alive := make(map[string]bool, len(latest))
	for h := range latest {
		alive[h] = !c.live.State(h, now).Down
	}
	var downs, ups []Alert
	for _, host := range hosts {
		key := fmt.Sprintf("down:%s", host)
		if !alive[host] {
			c.setDown(host, true)
			if c.state.ShouldAlert(key) {
				downs = append(downs, Alert{Host: host, Category: "down",
					Subject: fmt.Sprintf("[위험] %s 서버 연결 끊김", host),
					Body:    fmt.Sprintf("호스트: %s\n\n에이전트 연결이 끊겼습니다. 서버 상태를 확인하세요.", host)})
			}
		} else {
			// 패널은 7일 전 끊긴 서버도 보여 주므로 메일 발송 이력 없이 확인될 수 있다. 복귀하면 항상 풀어야 다음 장애 메일이 나간다
			c.state.ClearAck(host, "down", "critical")
			c.setDown(host, false)
			if c.state.ClearIfFiring(key) {
				ups = append(ups, Alert{Host: host, Category: "down",
					Subject: fmt.Sprintf("[복구] %s 서버 연결 복구", host),
					Body:    fmt.Sprintf("호스트: %s\n\n에이전트 연결이 복구되었습니다.", host)})
			}
		}
	}
	// 위쪽 장비 장애로 함께 끊긴 서버는 원인 알림에 묶고, 남은 것도 여러 대면 한 통으로
	c.SendDowns("[위험] 서버 %d대 연결 끊김", downs)
	c.SendUps("[복구] 서버 %d대 연결 복구", ups)
	c.reviewQuiet()
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
		// 판정 키가 호스트 단위라 마운트·시리즈별 결과 대신 호스트별 최댓값만 받음
		{"디스크", `max by (host_name)(delta(system_disk_usage_percent[1h]))`, 5, 15, "rapid_disk"},
		{"메모리", `max by (host_name)(delta(system_memory_usage_percent[1h]))`, 10, 25, "rapid_memory"},
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
				subject := fmt.Sprintf("[위험] %s 서비스 장애 (%s)", host, name)
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
	c.send(host, category, "", severity, subject, body)
}

// target은 같은 호스트·항목 안의 대상(디스크 마운트) — 한 마운트의 복구가 다른 마운트의 단계 통보 대기를 지우지 않게
func (c *Checker) send(host, category, target, severity, subject, body string) {
	cfg := c.config()
	if body, ok := c.admit(cfg, host, category, target, severity, subject, body); ok {
		c.notifier.Enqueue(routeNotice(cfg, c.defaultTo(), severity, subject, body))
	}
}

// admit은 점검·알림 꺼짐·확인 여부를 거르고 이력과 단계 통보 대기를 처리함. 보낼 알림이면 위치를 붙인 본문을 돌려줌
func (c *Checker) admit(cfg AlertConfig, host, category, target, severity, subject, body string) (string, bool) {
	if c.state.IsInMaintenance(host) {
		log.Printf("유지보수 모드 — 알림 억제: %s", subject)
		// 점검 중에 복구돼도 대기는 끝내야 점검을 끈 뒤 이미 끝난 장애가 2차 통보되지 않음
		if severity == "info" {
			c.trackUnacked(host, category, target, severity, subject, body)
		}
		return "", false
	}
	body = withLocation(body, c.location(host))
	switch {
	case !cfg.Enabled:
		// Synthetic·DPM·네트워크 장비·로그 분석은 check()의 스위치를 거치지 않고 들어옴
		log.Printf("알림 꺼짐 — 발송 생략: %s", subject)
	case severity != "info" && c.state.IsAcked(host, category, severity):
		log.Printf("알림 확인됨 — 발송 억제: %s", subject)
	default:
		c.trackUnacked(host, category, target, severity, subject, body)
		c.saveHistory(host, category, severity, subject, body)
		return body, true
	}
	c.saveHistory(host, category, severity, subject, body)
	return body, false
}

func (c *Checker) saveHistory(host, category, severity, subject, body string) {
	if c.history == nil {
		return
	}
	if err := c.history.Save(context.Background(), host, category, severity, subject, body); err != nil {
		log.Printf("알림 히스토리 저장 실패: %v", err)
	}
}

// 받는 곳은 알림 설정 값을 매번 반영 (공유 EmailConfig를 바꾸지 않음 — 보고서 고루틴과 경쟁)
func (c *Checker) defaultTo() []string {
	if c.email == nil {
		return nil
	}
	return c.email.To
}

type Alert struct {
	Host, Category, Subject, Body string
}

// 같은 주기에 이보다 많이 생기면 한 통으로 묶음
const groupAt = 4

// SendGroup은 같은 주기에 함께 생긴 알림이 많으면 한 통으로 보냄 — 상위 스위치 장애로 수백 대가 끊기면 장비마다 메일·문자가 나가 대기열과 문자 비용이 터짐.
// 이력·확인·단계 통보 대기는 장비별로 그대로 처리. subjectFmt의 %d에 실제 보낼 건수가 들어감
func (c *Checker) SendGroup(severity, subjectFmt string, alerts []Alert) {
	if len(alerts) < groupAt {
		for _, a := range alerts {
			c.sendAlert(a.Host, a.Category, severity, a.Subject, a.Body)
		}
		return
	}
	cfg := c.config()
	var lines []string
	for _, a := range alerts {
		if _, ok := c.admit(cfg, a.Host, a.Category, "", severity, a.Subject, a.Body); ok {
			line := "- " + a.Subject
			if loc := c.location(a.Host); loc != "" {
				line += " (" + loc + ")"
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return
	}
	subject := fmt.Sprintf(subjectFmt, len(lines))
	c.notifier.Enqueue(routeNotice(cfg, c.defaultTo(), severity, subject, strings.Join(lines, "\n")))
}

// SendAlert는 외부에서 알림을 발송할 수 있도록 sendAlert를 공개합니다.
// (synthetic, dpm 등 외부 모듈이 알림을 보낼 때 사용)
func (c *Checker) SendAlert(host, category, severity, subject, body string) {
	c.sendAlert(host, category, severity, subject, body)
}

// Ack는 알림을 확인 처리합니다.
func (c *Checker) Ack(host, category, severity, by string) {
	c.state.Ack(host, category, severity, by)
	// 주의 확인으로 다른 마운트의 위험 대기를 지우지 않게 위험 확인만 반영 — 위험→주의 하강은 checkDisk·SSL에서 정리
	if severity == "critical" {
		c.dropUnacked(func(p *pendingAlert) bool { return p.host == host && p.category == category })
	}
}

func (c *Checker) AckInfo(host, category, severity string) (AckInfo, bool) {
	return c.state.AckInfo(host, category, severity)
}

func (c *Checker) Acks() []AckInfo { return c.state.Acks() }

// 장애가 끝나거나 새로 시작될 때 풀어야 함 — 남아 있으면 다음 장애 메일이 억제됨
func (c *Checker) ClearAck(host, category, severity string) {
	c.state.ClearAck(host, category, severity)
	// 장애가 끝났거나(복구) 새 장애로 다시 시작되는 시점 — 지난 장애의 단계 통보 대기는 버림
	c.dropUnacked(func(p *pendingAlert) bool { return p.host == host && p.category == category })
}

// 끊긴 서버는 화면에 CPU·메모리·디스크 행이 없어 확인할 수 없음 — 끊긴 동안은 그 항목의 확인 대기 시간을 멈추고(대기는 유지) '끊김'으로만 확인받음.
// 대기를 지우면 짧은 수집 공백 뒤에 아직 위험인 항목이 재발송 쿨다운 동안 2차 통보되지 않고,
// 시간을 그대로 흘리면 오래 끊겼다 돌아온 직후 확인할 틈 없이 2차 통보됨
func (c *Checker) setDown(host string, down bool) {
	c.escMu.Lock()
	defer c.escMu.Unlock()
	since, was := c.downHosts[host]
	switch {
	case down && !was:
		c.downHosts[host] = time.Now()
	case !down && was:
		delete(c.downHosts, host)
		gap := time.Since(since)
		for _, p := range c.unacked {
			// 복귀한 주기에 새로 생긴 대기(끊긴 뒤 생성)는 그대로 — 끊기기 전 대기만 멈췄던 시간만큼 미룸
			if p.host == host && hostMetric(p.category) && p.at.Before(since) {
				p.at = p.at.Add(gap)
			}
		}
	}
}

func hostMetric(category string) bool {
	return category == "cpu" || category == "memory" || category == "disk"
}

// 화면에서 확인할 수 있고 복구 경로가 있는 항목만 단계 통보 — 나머지(이상탐지·급증·서비스·로그 분석)는 확인할 방법이 없어 늘 2차 통보가 나감
var escalatable = map[string]bool{"cpu": true, "memory": true, "disk": true, "down": true, "snmp": true, "synthetic": true, "ssl": true, "dpm": true}

// 위험 알림만 단계 통보 대상. 복구(info) 알림이 오면 같은 대상의 대기를 끝냄
func (c *Checker) trackUnacked(host, category, target, severity, subject, body string) {
	if !escalatable[category] {
		return
	}
	k := host + "/" + category + "/" + target
	c.escMu.Lock()
	defer c.escMu.Unlock()
	switch severity {
	case "info":
		delete(c.unacked, k)
	case "critical":
		if c.unacked[k] == nil {
			c.unacked[k] = &pendingAlert{host: host, category: category, target: target, subject: subject, body: body, at: time.Now()}
		}
	}
}

func (c *Checker) dropUnacked(match func(*pendingAlert) bool) {
	c.escMu.Lock()
	defer c.escMu.Unlock()
	for k, p := range c.unacked {
		if match(p) {
			delete(c.unacked, k)
		}
	}
}

// 오래 남은 대기는 복구 신호를 놓친 것으로 보고 버림 — 계속 쌓이지 않게. 단계 통보 시간보다는 길어야 함
const unackedTTL = 24 * time.Hour

// escalate는 확인되지 않고 정해진 시간이 지난 위험 알림을 2차 담당자에게 한 번 더 보냄. 같은 주기에 여러 건이면 한 통으로 묶음
func (c *Checker) escalate(cfg AlertConfig, now time.Time) {
	esc := cfg.Channels.Escalation
	after := time.Duration(esc.AfterMin) * time.Minute
	ttl := unackedTTL
	if after+time.Hour > ttl {
		ttl = after + time.Hour
	}
	c.escMu.Lock()
	var cands []string
	cats := map[string]bool{}
	for k, p := range c.unacked {
		switch {
		case c.state.IsAcked(p.host, p.category, "critical"):
			delete(c.unacked, k)
		case esc.AfterMin > 0 && !p.escalated && now.Sub(p.at) >= after && !c.state.IsInMaintenance(p.host) &&
			!(!c.downHosts[p.host].IsZero() && hostMetric(p.category)):
			cands = append(cands, k)
			cats[p.category] = true
		case now.Sub(p.at) > ttl && (c.downHosts[p.host].IsZero() || !hostMetric(p.category)):
			delete(c.unacked, k) // 끊긴 서버의 항목은 시계가 멈춘 것이라 TTL로 지우지 않음
		}
	}
	c.escMu.Unlock()
	if len(cands) == 0 {
		return
	}
	// DB 조회는 잠금 밖에서. 조회가 실패하면 감시 중으로 봄 (2차 통보를 놓치는 것보다 나음)
	names := map[string]map[string]bool{}
	for cat, fn := range c.watched {
		if !cats[cat] {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if m, err := fn(ctx); err == nil {
			names[cat] = m
		}
		cancel()
	}
	var due []pendingAlert
	c.escMu.Lock()
	for _, k := range cands {
		p := c.unacked[k]
		if p == nil || p.escalated {
			continue
		}
		if m, ok := names[p.category]; ok && !m[p.host] {
			delete(c.unacked, k)
			continue
		}
		p.escalated = true
		due = append(due, *p)
	}
	c.escMu.Unlock()
	if len(due) == 0 {
		return
	}
	sort.Slice(due, func(i, j int) bool { return due[i].at.Before(due[j].at) })
	m := Notice{Severity: "critical", Emails: esc.Recipients, SMS: cfg.Channels.SMS, Escalate: true}
	if g := cfg.Channels.SMS; g.Enabled && g.URL != "" {
		m.Phones = esc.Phones
	}
	tail := fmt.Sprintf("\n\n%d분 동안 아무도 확인하지 않아 2차 담당자에게 보냅니다.", esc.AfterMin)
	if len(due) == 1 {
		m.Subject = fmt.Sprintf("[미확인 %d분] %s", esc.AfterMin, due[0].subject)
		m.Body = due[0].body + tail
	} else {
		m.Subject = fmt.Sprintf("[미확인 %d분] 위험 알림 %d건", esc.AfterMin, len(due))
		lines := make([]string, len(due))
		for i, p := range due {
			lines[i] = "- " + p.subject
		}
		m.Body = strings.Join(lines, "\n") + tail
	}
	c.notifier.Enqueue(m)
	for _, p := range due {
		c.saveHistory(p.host, p.category, "critical", fmt.Sprintf("[미확인 %d분] %s", esc.AfterMin, p.subject), p.body+tail)
	}
}

// IsAcked는 알림이 확인됐는지 반환합니다.
func (c *Checker) IsAcked(host, category, severity string) bool {
	return c.state.IsAcked(host, category, severity)
}

// SetMaintenance는 호스트의 유지보수 모드를 설정합니다.
func (c *Checker) SetMaintenance(host string, on bool) {
	c.state.SetMaintenance(host, on)
	if on {
		// 점검 중엔 복구 알림도 억제돼 대기가 끝나지 않음 — 점검을 끈 뒤 지난 장애가 2차 통보되지 않게
		c.dropUnacked(func(p *pendingAlert) bool { return p.host == host })
	}
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
func (c *Checker) feedAnomalyDetector(cpu, mem []metricResult) {
	now := time.Now()
	for _, r := range cpu {
		host := r.metric["host_name"]
		if a := c.Detector.Feed(host, "cpu", r.value, now); a != nil {
			key := fmt.Sprintf("anomaly:cpu:%s", host)
			if a.Severity == "critical" && c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("[위험] %s CPU 이상 감지 (Z=%.1f)", host, a.ZScore)
				c.sendAlert(host, "anomaly_cpu", a.Severity, subject, a.Message)
			}
		}
	}
	for _, r := range mem {
		host := r.metric["host_name"]
		if a := c.Detector.Feed(host, "memory", r.value, now); a != nil {
			key := fmt.Sprintf("anomaly:memory:%s", host)
			if a.Severity == "critical" && c.state.ShouldAlert(key) {
				subject := fmt.Sprintf("[위험] %s 메모리 이상 감지 (Z=%.1f)", host, a.ZScore)
				c.sendAlert(host, "anomaly_memory", a.Severity, subject, a.Message)
			}
		}
	}
}

// feedDiskPredictor는 디스크 사용률을 예측 엔진에 주입합니다.
func (c *Checker) feedDiskPredictor(results []metricResult) {
	now := time.Now()
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
