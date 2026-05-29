package dpm

import (
	"context"
	"log"
	"sync"
	"time"
)

// InstanceLister는 등록된 인스턴스 목록을 제공합니다.
type InstanceLister interface {
	List(ctx context.Context) ([]Instance, error)
}

// ResultSaver는 폴링 결과를 저장합니다.
type ResultSaver interface {
	SaveMetrics(ctx context.Context, m InstanceMetrics) error
	SaveQueryStats(ctx context.Context, stats []QueryStat) error
}

// Poller는 등록된 인스턴스를 각각의 주기로 폴링합니다.
type Poller struct {
	lister InstanceLister
	saver  ResultSaver
	cipher *Cipher

	mu     sync.RWMutex
	latest map[int64]PollResult
	stops  map[int64]chan struct{}

	// 알림 콜백
	onAlert func(inst Instance, severity, subject, body string)
}

func NewPoller(lister InstanceLister, saver ResultSaver, cipher *Cipher) *Poller {
	return &Poller{
		lister: lister,
		saver:  saver,
		cipher: cipher,
		latest: make(map[int64]PollResult),
		stops:  make(map[int64]chan struct{}),
	}
}

// SetAlertCallback은 알림 콜백을 설정합니다.
func (p *Poller) SetAlertCallback(fn func(inst Instance, severity, subject, body string)) {
	p.onAlert = fn
}

// Start는 5분마다 인스턴스 목록을 동기화하면서 각 인스턴스의 폴링 루프를 관리합니다.
func (p *Poller) Start(ctx context.Context) {
	p.syncInstances(ctx)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				p.stopAll()
				return
			case <-ticker.C:
				p.syncInstances(ctx)
			}
		}
	}()
}

func (p *Poller) syncInstances(ctx context.Context) {
	instances, err := p.lister.List(ctx)
	if err != nil {
		log.Printf("DPM: 인스턴스 목록 조회 실패: %v", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	active := make(map[int64]bool)
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		active[inst.ID] = true
		if _, ok := p.stops[inst.ID]; !ok {
			stop := make(chan struct{})
			p.stops[inst.ID] = stop
			go p.runInstance(inst, stop)
		}
	}
	for id, stop := range p.stops {
		if !active[id] {
			close(stop)
			delete(p.stops, id)
			delete(p.latest, id)
		}
	}
}

func (p *Poller) runInstance(inst Instance, stop chan struct{}) {
	interval := time.Duration(inst.PollIntervalSec) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	p.pollOnce(inst)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			p.pollOnce(inst)
		}
	}
}

func (p *Poller) pollOnce(inst Instance) {
	password, err := p.cipher.Decrypt(inst.PasswordEnc)
	if err != nil {
		log.Printf("DPM: 비밀번호 복호화 실패 (instance=%d): %v", inst.ID, err)
		return
	}

	ctx := context.Background()
	var result PollResult
	switch inst.DBType {
	case "postgres":
		result = CollectPostgres(ctx, inst, password, 20)
	default:
		log.Printf("DPM: 미지원 DB 타입: %s", inst.DBType)
		return
	}

	_ = p.saver.SaveMetrics(ctx, result.Metrics)
	if len(result.TopQueries) > 0 {
		_ = p.saver.SaveQueryStats(ctx, result.TopQueries)
	}

	p.mu.Lock()
	prev, hadPrev := p.latest[inst.ID]
	p.latest[inst.ID] = result
	p.mu.Unlock()

	// 알림 평가
	p.evaluateAlerts(inst, result, prev, hadPrev)
}

// evaluateAlerts는 임계치 기반 알림을 발송합니다.
func (p *Poller) evaluateAlerts(inst Instance, cur PollResult, prev PollResult, hadPrev bool) {
	if p.onAlert == nil {
		return
	}

	// 1) 연결 실패
	if cur.Metrics.Error != "" && (!hadPrev || prev.Metrics.Error == "") {
		p.onAlert(inst, "critical",
			"[심각] [DPM] "+inst.Name+" DB 연결 실패",
			"인스턴스: "+inst.Name+"\n호스트: "+inst.Host+"\n오류: "+cur.Metrics.Error,
		)
		return
	}
	if cur.Metrics.Error != "" {
		return // 연결 실패 상태에선 추가 알림 평가 안 함
	}

	// 2) 커넥션 80% 초과
	if cur.Metrics.ConnectionsMax > 0 {
		used := cur.Metrics.ConnectionsActive + cur.Metrics.ConnectionsIdle
		ratio := float64(used) / float64(cur.Metrics.ConnectionsMax)
		if ratio >= 0.8 {
			p.onAlert(inst, "warning",
				"[주의] [DPM] "+inst.Name+" 커넥션 임계치 초과",
				"사용 중: "+itoa(used)+" / "+itoa(cur.Metrics.ConnectionsMax)+
					" ("+ftoa(ratio*100, 1)+"%)",
			)
		}
	}

	// 3) 캐시 히트율 90% 미만
	if cur.Metrics.CacheHitRatio > 0 && cur.Metrics.CacheHitRatio < 0.9 {
		p.onAlert(inst, "warning",
			"[주의] [DPM] "+inst.Name+" 캐시 히트율 저하",
			"히트율: "+ftoa(cur.Metrics.CacheHitRatio*100, 2)+"% (권장: 90% 이상)",
		)
	}
}

func (p *Poller) stopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, stop := range p.stops {
		close(stop)
		delete(p.stops, id)
	}
}

// GetLatest는 모든 인스턴스의 최근 폴링 결과를 반환합니다.
func (p *Poller) GetLatest() map[int64]PollResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[int64]PollResult, len(p.latest))
	for k, v := range p.latest {
		out[k] = v
	}
	return out
}

// TriggerOne은 단일 인스턴스를 즉시 폴링합니다.
func (p *Poller) TriggerOne(inst Instance) PollResult {
	p.pollOnce(inst)
	p.mu.RLock()
	r := p.latest[inst.ID]
	p.mu.RUnlock()
	return r
}

// Cipher는 외부에서 비밀번호 암호화 시 사용합니다.
func (p *Poller) Cipher() *Cipher { return p.cipher }
