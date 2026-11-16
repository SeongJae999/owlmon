// Package snmp는 SNMP v2c로 네트워크 장비를 폴링합니다.
package snmp

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// OID 상수
const (
	oidSysName    = "1.3.6.1.2.1.1.5.0"
	oidSysUpTime  = "1.3.6.1.2.1.1.3.0"
	oidIfDescr    = "1.3.6.1.2.1.2.2.1.2"     // 인터페이스 이름 테이블
	oidIfOperStat = "1.3.6.1.2.1.2.2.1.8"     // 운영 상태 (1=up, 2=down)
	oidIfInOctets = "1.3.6.1.2.1.2.2.1.10"    // 수신 바이트 (32bit)
	oidIfOutOctet = "1.3.6.1.2.1.2.2.1.16"    // 송신 바이트 (32bit)
	oidHCIn       = "1.3.6.1.2.1.31.1.1.1.6"  // 수신 바이트 (64bit)
	oidHCOut      = "1.3.6.1.2.1.31.1.1.1.10" // 송신 바이트 (64bit)
)

// Device는 SNMP 장비 설정입니다.
type Device struct {
	ID        int64
	Name      string
	IP        string
	Community string `json:"-"` // 장비 접속 암호이므로 어떤 API 응답에도 내보내지 않음
	Port      int
	// 서버 직접 폴링이 응답받은 적 있음(DB에 남김) — 재기동 뒤에도 서버 폴링 실패를 장애 근거로 씀
	ServerReach bool `json:"-"`
}

// InterfaceStats는 단일 인터페이스 상태입니다.
type InterfaceStats struct {
	Index    int
	Name     string
	OperUp   bool // true = up
	InBytes  uint64
	OutBytes uint64
	InBps    float64 // bytes/sec (delta)
	OutBps   float64 // bytes/sec (delta)
}

// DeviceStatus는 장비 전체 상태입니다.
type DeviceStatus struct {
	Device      Device
	Up          bool
	UptimeSec   float64
	Interfaces  []InterfaceStats
	CollectedAt time.Time
	LastError   string     `json:",omitempty"` // 폴링 실패 시 사람이 읽을 수 있는 메시지
	DownSince   *time.Time `json:",omitempty"` // 응답이 끊기기 시작한 시각 (상황판 "N분째" 표시)
	ResponseMs  int64      // 폴링 소요 시간 (ms)
}

// ifCounter는 이전 폴링 값(delta 계산용)을 저장합니다.
type ifCounter struct {
	in, out uint64
	at      time.Time
}

// Poller는 SNMP 장비를 주기적으로 폴링합니다.
type Poller struct {
	mu       sync.RWMutex
	statuses map[int64]*DeviceStatus // device ID → 최신 상태
	counters map[string]*ifCounter   // "deviceID:ifIndex" → 이전 카운터
	inflight map[int64]bool          // 폴링 진행 중인 장비 (느린 장비 중복 폴링 → 고루틴 누적 방지)
	watch    map[int64]*downWatch    // 장애 알림용 판정 (Observe)
	slots    chan struct{}           // 동시 폴링 상한 — 30초마다 장비 수만큼 UDP 소켓이 한꺼번에 열리지 않게
}

// 응답 없는 장비는 한 번에 약 6초 자리를 잡음 — 3,000대가 2분(판정 근거 유효 시간) 안에 한 바퀴 돌 만큼
const maxConcurrentPolls = 256

type downWatch struct {
	name     string // 이름이 바뀌면 옛 이름으로 걸린 알림·확인을 풀어야 함
	streak   int    // 연속 무응답
	upStreak int    // 장애 알림 뒤 연속 응답
	since    time.Time
	alerted  bool
	// 마지막으로 센 샘플 시각 — 같은 샘플을 주기마다 다시 세면 '연속 두 번'이 한 번 유실로 채워짐.
	// 출처별로 따로 비교 — 서버 폴링 시각이 흘렀다고 같은 에이전트 샘플을 다시 세지 않게
	agentAt, serverAt time.Time
	serverReach       bool // 서버 직접 폴링이 응답받은 적 있음 — 서버가 닿는 장비라 서버 폴링 실패가 장애 근거가 됨
	noData            bool // 직전 판정에 근거가 없었음(수집 경로 끊김) — 화면이 '응답 없음' 대신 '수집 끊김'으로 보이게
	conflict          bool // 서버 폴링은 실패인데 에이전트 값은 응답 중 — 서버 쪽 경로 문제일 수도 있어 알림 문구로 구분
	noDataSince       time.Time
	noDataAlerted     bool
}

// 장애 판정 기록만 초기화 — 샘플 시각·도달 여부는 유지
func (w *downWatch) reset() {
	w.streak, w.upStreak, w.since, w.alerted = 0, 0, time.Time{}, false
}

func NewPoller() *Poller {
	return &Poller{
		statuses: make(map[int64]*DeviceStatus),
		inflight: make(map[int64]bool),
		counters: make(map[string]*ifCounter),
		watch:    make(map[int64]*downWatch),
		slots:    make(chan struct{}, maxConcurrentPolls),
	}
}

// 한 번 놓친 응답(UDP 유실)으로 알림이 나가지 않게 연속 두 번(약 1분) 응답이 없어야 장애로 봄.
// 복구도 연속 두 번 — 링크가 불안정한 장비가 주기마다 위험·복구를 반복해 보내지 않게
const (
	downAfter = 2
	upAfter   = 2
)

// Event는 장애 시작·복구 전환. Removed는 장애 중에 목록에서 빠지거나 이름이 바뀐 장비 — 복구 알림 없이 옛 이름의 알림만 정리
type Event struct {
	Device   Device
	Down     bool // true = 장애 시작, false = 복구
	Removed  bool
	NoData   bool      // 수집 끊김이 noDataAlertAfter 넘게 이어짐 — 상태를 모르는 채로 알림이 영영 안 나가지 않게
	Conflict bool      // 장애 근거가 서버 폴링뿐이고 에이전트 값은 응답 중
	Since    time.Time // 응답이 끊기기 시작한 시각
}

const noDataAlertAfter = 15 * time.Minute

// 서버 직접 폴링 결과가 이보다 오래되면 응답 근거로 쓰지 않음
const memFresh = 2 * time.Minute

// Up은 화면·알림이 같은 기준을 쓰도록 한 곳에서 판정 — 에이전트가 수집한 값(Prometheus) 또는 서버 직접 폴링 중 하나라도 응답하면 정상.
// 단 서버가 닿는 장비의 서버 폴링 실패는 에이전트 값보다 우선 — 에이전트 키는 공유라 누구나 장비 응답을 보낼 수 있음
func (p *Poller) Up(dev Device, promUp map[string]bool, now time.Time) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	st := p.statuses[dev.ID]
	fresh := st != nil && now.Sub(st.CollectedAt) < memFresh
	if fresh && st.Up {
		return true
	}
	if fresh && p.reached(dev) {
		return false
	}
	return promUp[dev.Name]
}

func (p *Poller) reached(dev Device) bool {
	w := p.watch[dev.ID]
	return dev.ServerReach || (w != nil && w.serverReach)
}

// Reached는 서버 직접 폴링이 응답받은 적 있는 장비 — 호출 측이 DB에 남겨 재기동·이름 변경 뒤에도 유지
func (p *Poller) Reached(dev Device) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.reached(dev)
}

// Sample은 에이전트가 수집한 장비 응답 (Prometheus snmp_device_up 최신 샘플)
type Sample struct {
	Up bool
	At time.Time
}

// evidence는 응답 여부와 근거가 된 샘플 시각(출처별)을 돌려줌. 둘 다 0이면 근거 없음. 호출 측이 p.mu를 잡고 있어야 함.
// 서버 폴링 실패는 서버가 닿는 장비(응답받은 적 있음)거나 에이전트 수집 기록이 없는 장비일 때만 근거 —
// 에이전트가 오래 꺼져도 서버가 못 닿는 망분리 장비 전부가 장애로 나가지 않게
func (p *Poller) evidence(dev Device, w *downWatch, latest map[string]Sample, recent map[string]bool, now time.Time) (up bool, agentAt, serverAt time.Time) {
	s, inProm := latest[dev.Name]
	st := p.statuses[dev.ID]
	fresh := st != nil && now.Sub(st.CollectedAt) < memFresh
	if dev.ServerReach || (fresh && st.Up) {
		w.serverReach = true
	}
	serverDown := fresh && !st.Up && w.serverReach
	w.conflict = serverDown && inProm && s.Up
	switch {
	case inProm && s.Up && !serverDown:
		return true, s.At, time.Time{}
	case fresh && st.Up:
		return true, time.Time{}, st.CollectedAt
	}
	if inProm && !s.Up {
		agentAt = s.At
	}
	if fresh && (w.serverReach || !recent[dev.Name]) {
		serverAt = st.CollectedAt
	}
	return false, agentAt, serverAt
}

// Observe는 감시 주기마다 불려 장애 시작·복구 전환만 돌려줌. 목록에서 빠진 장비의 기록은 지움
func (p *Poller) Observe(devices []Device, latest map[string]Sample, recent map[string]bool, now time.Time) []Event {
	var events []Event
	seen := map[int64]bool{}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, d := range devices {
		seen[d.ID] = true
		w := p.watch[d.ID]
		if w != nil && w.name != d.Name {
			if w.alerted {
				events = append(events, Event{Device: Device{ID: d.ID, Name: w.name, IP: d.IP}, Removed: true, Since: w.since})
			}
			w = nil
		}
		if w == nil {
			w = &downWatch{name: d.Name}
			p.watch[d.ID] = w
		}
		up, agentAt, serverAt := p.evidence(d, w, latest, recent, now)
		w.noData = !up && agentAt.IsZero() && serverAt.IsZero()
		switch {
		case !w.noData:
			w.noDataSince, w.noDataAlerted = time.Time{}, false
		case w.noDataSince.IsZero():
			w.noDataSince = now
		case !w.noDataAlerted && now.Sub(w.noDataSince) >= noDataAlertAfter:
			w.noDataAlerted = true
			events = append(events, Event{Device: d, NoData: true, Since: w.noDataSince})
		}
		newAgent, newServer := agentAt.After(w.agentAt), serverAt.After(w.serverAt)
		if newAgent {
			w.agentAt = agentAt
		}
		if newServer {
			w.serverAt = serverAt
		}
		switch {
		case !newAgent && !newServer:
			// 근거 없음이거나 새 샘플 없음 — 연속 횟수를 그대로 둠 (수집 주기가 감시 주기보다 길거나 폴링이 밀림)
		case up && w.alerted:
			w.upStreak++
			if w.upStreak >= upAfter {
				events = append(events, Event{Device: d, Down: false, Since: w.since})
				w.reset()
			}
		case up:
			w.reset()
		default:
			w.upStreak = 0
			if w.streak == 0 {
				w.since = now
			}
			w.streak++
			if w.streak >= downAfter && !w.alerted {
				w.alerted = true
				events = append(events, Event{Device: d, Down: true, Conflict: w.conflict, Since: w.since})
			}
		}
	}
	for id, w := range p.watch {
		if !seen[id] {
			if w.alerted {
				events = append(events, Event{Device: Device{ID: id, Name: w.name}, Removed: true, Since: w.since})
			}
			delete(p.watch, id)
		}
	}
	return events
}

// NoData는 직전 판정에 근거가 없던 장비 — 에이전트 수집이 끊겼고 서버도 닿지 않아 상태를 모름
func (p *Poller) NoData(id int64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	w := p.watch[id]
	return w != nil && w.noData
}

// DownSince는 응답이 끊긴 장비의 시작 시각 (응답 중이면 nil) — 화면의 "N분째"
func (p *Poller) DownSince(id int64) *time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if w := p.watch[id]; w != nil && w.streak > 0 {
		t := w.since
		return &t
	}
	return nil
}

// Poll은 단일 장비를 폴링하고 내부 상태를 갱신합니다.
func (p *Poller) Poll(dev Device) {
	p.mu.Lock()
	if p.inflight[dev.ID] {
		p.mu.Unlock()
		return // 이전 폴링이 아직 진행 중 — 이번 주기는 건너뜀
	}
	p.inflight[dev.ID] = true
	p.mu.Unlock()

	// 자리를 기다리는 동안도 inflight라 다음 주기가 같은 장비를 또 줄 세우지 않음
	p.slots <- struct{}{}
	status := p.poll(dev)
	<-p.slots
	p.mu.Lock()
	if !status.Up {
		since := status.CollectedAt
		if prev := p.statuses[dev.ID]; prev != nil && !prev.Up && prev.DownSince != nil {
			since = *prev.DownSince
		}
		status.DownSince = &since
	}
	p.statuses[dev.ID] = status
	delete(p.inflight, dev.ID)
	p.mu.Unlock()
}

// Statuses는 모든 장비의 최신 상태를 반환합니다.
func (p *Poller) Statuses() []*DeviceStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*DeviceStatus, 0, len(p.statuses))
	for _, s := range p.statuses {
		out = append(out, s)
	}
	return out
}

// Status는 특정 장비의 최신 상태를 반환합니다.
func (p *Poller) Status(deviceID int64) *DeviceStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.statuses[deviceID]
}

func (p *Poller) poll(dev Device) *DeviceStatus {
	start := time.Now()
	status := &DeviceStatus{Device: dev, CollectedAt: start}
	defer func() { status.ResponseMs = time.Since(start).Milliseconds() }()

	g := &gosnmp.GoSNMP{
		Target:    dev.IP,
		Port:      uint16(dev.Port),
		Community: dev.Community,
		Version:   gosnmp.Version2c,
		Timeout:   3 * time.Second,
		Retries:   1,
	}
	if err := g.Connect(); err != nil {
		status.LastError = fmt.Sprintf("연결 실패: %v", err)
		log.Printf("[SNMP] %s(%s) 연결 실패: %v", dev.Name, dev.IP, err)
		return status
	}
	defer g.Conn.Close()

	// 시스템 기본 정보
	sysResult, err := g.Get([]string{oidSysName, oidSysUpTime})
	if err != nil {
		status.LastError = fmt.Sprintf("SNMP 조회 실패 (community/방화벽 확인): %v", err)
		log.Printf("[SNMP] %s(%s) 조회 실패: %v", dev.Name, dev.IP, err)
		return status
	}
	status.Up = true
	for _, v := range sysResult.Variables {
		switch v.Name {
		case "." + oidSysUpTime:
			if ticks, ok := v.Value.(uint32); ok {
				status.UptimeSec = float64(ticks) / 100
			}
		}
	}

	// 인터페이스 테이블 Walk
	ifNames := map[int]string{}
	ifStats := map[int]*InterfaceStats{}

	// ifDescr Walk
	g.Walk(oidIfDescr, func(pdu gosnmp.SnmpPDU) error {
		idx := ifIndex(pdu.Name, oidIfDescr)
		if idx > 0 {
			name := strings.TrimSpace(fmt.Sprintf("%s", pdu.Value))
			ifNames[idx] = name
			ifStats[idx] = &InterfaceStats{Index: idx, Name: name}
		}
		return nil
	})

	// ifOperStatus Walk
	g.Walk(oidIfOperStat, func(pdu gosnmp.SnmpPDU) error {
		idx := ifIndex(pdu.Name, oidIfOperStat)
		if s, ok := ifStats[idx]; ok {
			s.OperUp = gosnmp.ToBigInt(pdu.Value).Int64() == 1
		}
		return nil
	})

	// 64bit 카운터 우선 시도 (HCOctets)
	hcSupported := true
	g.Walk(oidHCIn, func(pdu gosnmp.SnmpPDU) error {
		idx := ifIndex(pdu.Name, oidHCIn)
		if s, ok := ifStats[idx]; ok {
			s.InBytes = uint64(gosnmp.ToBigInt(pdu.Value).Int64())
		}
		return nil
	})
	if len(ifStats) > 0 {
		allZero := true
		for _, s := range ifStats {
			if s.InBytes > 0 {
				allZero = false
				break
			}
		}
		hcSupported = !allZero
	}

	if hcSupported {
		g.Walk(oidHCOut, func(pdu gosnmp.SnmpPDU) error {
			idx := ifIndex(pdu.Name, oidHCOut)
			if s, ok := ifStats[idx]; ok {
				s.OutBytes = uint64(gosnmp.ToBigInt(pdu.Value).Int64())
			}
			return nil
		})
	} else {
		// 32bit 폴백
		g.Walk(oidIfInOctets, func(pdu gosnmp.SnmpPDU) error {
			idx := ifIndex(pdu.Name, oidIfInOctets)
			if s, ok := ifStats[idx]; ok {
				s.InBytes = uint64(gosnmp.ToBigInt(pdu.Value).Int64())
			}
			return nil
		})
		g.Walk(oidIfOutOctet, func(pdu gosnmp.SnmpPDU) error {
			idx := ifIndex(pdu.Name, oidIfOutOctet)
			if s, ok := ifStats[idx]; ok {
				s.OutBytes = uint64(gosnmp.ToBigInt(pdu.Value).Int64())
			}
			return nil
		})
	}

	// delta 계산 (bytes/sec)
	now := time.Now()
	for idx, s := range ifStats {
		key := fmt.Sprintf("%d:%d", dev.ID, idx)
		p.mu.Lock()
		prev, hasPrev := p.counters[key]
		if hasPrev {
			elapsed := now.Sub(prev.at).Seconds()
			if elapsed > 0 && s.InBytes >= prev.in && s.OutBytes >= prev.out {
				s.InBps = float64(s.InBytes-prev.in) / elapsed
				s.OutBps = float64(s.OutBytes-prev.out) / elapsed
			}
		}
		p.counters[key] = &ifCounter{in: s.InBytes, out: s.OutBytes, at: now}
		p.mu.Unlock()
	}

	for _, s := range ifStats {
		status.Interfaces = append(status.Interfaces, *s)
	}
	return status
}

// ifIndex는 OID에서 인터페이스 인덱스를 파싱합니다.
// 예: ".1.3.6.1.2.1.2.2.1.2.5" → 5
func ifIndex(oid, base string) int {
	suffix := strings.TrimPrefix(oid, "."+base+".")
	if suffix == oid {
		return -1
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return -1
	}
	return idx
}
