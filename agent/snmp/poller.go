// Package snmp는 에이전트가 사내 SNMP 장비를 폴링해서 OTLP 메트릭으로 변환합니다.
// 학교/공공기관 망분리 환경에서 OWLmon 서버가 직접 SNMP 못 닿을 때 사용.
package snmp

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// Device는 폴링 대상 장비 한 대입니다.
type Device struct {
	Name      string
	IP        string
	Community string
	Port      int
	Type      string // printer / switch / generic
}

// DeviceMetrics는 폴링 1회 결과를 담습니다.
type DeviceMetrics struct {
	Device      Device
	Up          bool          // SNMP 응답 여부
	UptimeSec   float64       // sysUpTime
	SysDescr    string        // 장비 설명 (KONICA MINOLTA bizhub C250i 같은)
	ResponseMs  int64         // 폴링 소요 시간
	LastError   string        // 실패 시 에러
	CollectedAt time.Time

	// 프린터 전용
	Supplies   []SupplyLevel  // 토너/잉크 잔량
	TotalPages int64          // 누적 인쇄 페이지

	// 스위치 전용
	Interfaces []InterfaceStats
}

// SupplyLevel은 토너/잉크 1개의 상태입니다.
type SupplyLevel struct {
	Index       int
	Description string // "Black Toner", "Cyan Toner" 같은
	Level       int    // 현재 잔량 (단위 = MaxCapacity 단위와 동일, 보통 16 = 어떤 단위)
	MaxCapacity int    // 최대 용량 (-2 = 알 수 없음, -3 = 무한)
	PercentLeft float64 // 0~100 (계산 결과)
}

// InterfaceStats는 인터페이스 1개의 상태입니다.
type InterfaceStats struct {
	Index    int
	Name     string
	OperUp   bool
	InBytes  uint64
	OutBytes uint64
}

// 표준 OID
const (
	oidSysDescr     = "1.3.6.1.2.1.1.1.0"
	oidSysUpTime    = "1.3.6.1.2.1.1.3.0"
	oidSysName      = "1.3.6.1.2.1.1.5.0"

	// Printer-MIB (RFC 3805)
	oidPrtMarkerSuppliesDescr   = "1.3.6.1.2.1.43.11.1.1.6"  // walk → 토너 이름
	oidPrtMarkerSuppliesLevel   = "1.3.6.1.2.1.43.11.1.1.9"  // walk → 현재 잔량
	oidPrtMarkerSuppliesMaxCap  = "1.3.6.1.2.1.43.11.1.1.8"  // walk → 최대 용량
	oidPrtMarkerLifeCount       = "1.3.6.1.2.1.43.10.2.1.4"  // walk → 총 인쇄 페이지

	// IF-MIB (스위치/라우터 공용)
	oidIfDescr     = "1.3.6.1.2.1.2.2.1.2"
	oidIfOperStat  = "1.3.6.1.2.1.2.2.1.8"
	oidHCIn        = "1.3.6.1.2.1.31.1.1.1.6"
	oidHCOut       = "1.3.6.1.2.1.31.1.1.1.10"
)

// Poller는 N대의 장비를 주기적으로 폴링하고 최신 상태를 메모리에 보관합니다.
type Poller struct {
	devices []Device

	mu      sync.RWMutex
	results map[string]*DeviceMetrics // device.Name → 최신 결과
}

func NewPoller(devices []Device) *Poller {
	return &Poller{
		devices: devices,
		results: make(map[string]*DeviceMetrics),
	}
}

// PollAll은 모든 장비를 한 번 폴링합니다 (각 장비를 goroutine으로).
func (p *Poller) PollAll() {
	var wg sync.WaitGroup
	for _, dev := range p.devices {
		wg.Add(1)
		go func(d Device) {
			defer wg.Done()
			m := p.pollOne(d)
			p.mu.Lock()
			p.results[d.Name] = m
			p.mu.Unlock()
		}(dev)
	}
	wg.Wait()
}

// Results는 모든 장비의 최신 결과 복사본을 반환합니다.
func (p *Poller) Results() []DeviceMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]DeviceMetrics, 0, len(p.results))
	for _, r := range p.results {
		out = append(out, *r)
	}
	return out
}

// Get은 특정 장비의 최신 결과를 반환합니다 (없으면 nil).
func (p *Poller) Get(name string) *DeviceMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if r, ok := p.results[name]; ok {
		cp := *r
		return &cp
	}
	return nil
}

func (p *Poller) pollOne(dev Device) *DeviceMetrics {
	start := time.Now()
	m := &DeviceMetrics{Device: dev, CollectedAt: start}
	defer func() { m.ResponseMs = time.Since(start).Milliseconds() }()

	g := &gosnmp.GoSNMP{
		Target:    dev.IP,
		Port:      uint16(dev.Port),
		Community: dev.Community,
		Version:   gosnmp.Version2c,
		Timeout:   3 * time.Second,
		Retries:   1,
	}
	if err := g.Connect(); err != nil {
		m.LastError = fmt.Sprintf("연결 실패: %v", err)
		return m
	}
	defer g.Conn.Close()

	// 1) 시스템 정보 (sysDescr + sysUpTime)
	sys, err := g.Get([]string{oidSysDescr, oidSysUpTime})
	if err != nil {
		m.LastError = fmt.Sprintf("SNMP get 실패 (community/방화벽 확인): %v", err)
		return m
	}
	m.Up = true
	for _, v := range sys.Variables {
		switch v.Name {
		case "." + oidSysDescr:
			if b, ok := v.Value.([]byte); ok {
				m.SysDescr = strings.TrimSpace(string(b))
			} else {
				m.SysDescr = fmt.Sprintf("%v", v.Value)
			}
		case "." + oidSysUpTime:
			if ticks, ok := v.Value.(uint32); ok {
				m.UptimeSec = float64(ticks) / 100.0
			}
		}
	}

	// 2) 장비 타입별 분기
	switch dev.Type {
	case "printer":
		p.pollPrinter(g, m)
	case "switch":
		p.pollSwitch(g, m)
	}
	return m
}

// pollPrinter는 Printer-MIB(RFC 3805) 핵심 OID를 수집합니다.
func (p *Poller) pollPrinter(g *gosnmp.GoSNMP, m *DeviceMetrics) {
	// 토너/잉크 잔량 — 세 컬럼 walk 후 인덱스 매칭
	descrs := map[int]string{}
	levels := map[int]int{}
	maxCaps := map[int]int{}

	_ = g.Walk(oidPrtMarkerSuppliesDescr, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		if b, ok := pdu.Value.([]byte); ok {
			descrs[idx] = strings.TrimSpace(string(b))
		}
		return nil
	})
	_ = g.Walk(oidPrtMarkerSuppliesLevel, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		levels[idx] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		return nil
	})
	_ = g.Walk(oidPrtMarkerSuppliesMaxCap, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		maxCaps[idx] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		return nil
	})

	for idx, descr := range descrs {
		s := SupplyLevel{Index: idx, Description: descr}
		if lv, ok := levels[idx]; ok {
			s.Level = lv
		}
		if mc, ok := maxCaps[idx]; ok {
			s.MaxCapacity = mc
		}
		// MaxCap > 0 일 때만 % 계산. 음수면 unknown(-2) 또는 무한(-3) — Printer-MIB 정의
		if s.MaxCapacity > 0 {
			s.PercentLeft = float64(s.Level) / float64(s.MaxCapacity) * 100.0
			if s.PercentLeft < 0 {
				s.PercentLeft = 0
			} else if s.PercentLeft > 100 {
				s.PercentLeft = 100
			}
		}
		m.Supplies = append(m.Supplies, s)
	}

	// 총 인쇄 페이지 (Marker life count — 여러 marker 합산)
	_ = g.Walk(oidPrtMarkerLifeCount, func(pdu gosnmp.SnmpPDU) error {
		m.TotalPages += gosnmp.ToBigInt(pdu.Value).Int64()
		return nil
	})
}

// pollSwitch는 IF-MIB 핵심 OID를 수집합니다 (NETGEAR / 일반 스위치 공용).
func (p *Poller) pollSwitch(g *gosnmp.GoSNMP, m *DeviceMetrics) {
	names := map[int]string{}
	ifs := map[int]*InterfaceStats{}

	_ = g.Walk(oidIfDescr, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		if b, ok := pdu.Value.([]byte); ok {
			names[idx] = strings.TrimSpace(string(b))
			ifs[idx] = &InterfaceStats{Index: idx, Name: names[idx]}
		}
		return nil
	})
	_ = g.Walk(oidIfOperStat, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		if s, ok := ifs[idx]; ok {
			s.OperUp = gosnmp.ToBigInt(pdu.Value).Int64() == 1
		}
		return nil
	})
	_ = g.Walk(oidHCIn, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		if s, ok := ifs[idx]; ok {
			s.InBytes = uint64(gosnmp.ToBigInt(pdu.Value).Int64())
		}
		return nil
	})
	_ = g.Walk(oidHCOut, func(pdu gosnmp.SnmpPDU) error {
		idx := lastIndex(pdu.Name)
		if s, ok := ifs[idx]; ok {
			s.OutBytes = uint64(gosnmp.ToBigInt(pdu.Value).Int64())
		}
		return nil
	})
	for _, s := range ifs {
		m.Interfaces = append(m.Interfaces, *s)
	}
}

// lastIndex는 OID 마지막 숫자(인덱스)를 파싱합니다.
// 예: ".1.3.6.1.2.1.43.11.1.1.6.1.5" → 5
func lastIndex(oid string) int {
	parts := strings.Split(oid, ".")
	if len(parts) == 0 {
		return -1
	}
	last := parts[len(parts)-1]
	var n int
	if _, err := fmt.Sscanf(last, "%d", &n); err != nil {
		return -1
	}
	return n
}

// Start는 PollInterval 주기로 PollAll을 무한 반복하는 goroutine을 띄웁니다.
func (p *Poller) Start(interval time.Duration) {
	go func() {
		// 첫 폴링 즉시 1회
		p.PollAll()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			p.PollAll()
		}
	}()
	log.Printf("[SNMP] 폴링 시작: 장비 %d대, 주기 %s", len(p.devices), interval)
}
