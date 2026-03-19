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
	oidIfDescr    = "1.3.6.1.2.1.2.2.1.2"    // 인터페이스 이름 테이블
	oidIfOperStat = "1.3.6.1.2.1.2.2.1.8"    // 운영 상태 (1=up, 2=down)
	oidIfInOctets = "1.3.6.1.2.1.2.2.1.10"   // 수신 바이트 (32bit)
	oidIfOutOctet = "1.3.6.1.2.1.2.2.1.16"   // 송신 바이트 (32bit)
	oidHCIn       = "1.3.6.1.2.1.31.1.1.1.6" // 수신 바이트 (64bit)
	oidHCOut      = "1.3.6.1.2.1.31.1.1.1.10" // 송신 바이트 (64bit)
)

// Device는 SNMP 장비 설정입니다.
type Device struct {
	ID        int64
	Name      string
	IP        string
	Community string
	Port      int
}

// InterfaceStats는 단일 인터페이스 상태입니다.
type InterfaceStats struct {
	Index     int
	Name      string
	OperUp    bool    // true = up
	InBytes   uint64
	OutBytes  uint64
	InBps     float64 // bytes/sec (delta)
	OutBps    float64 // bytes/sec (delta)
}

// DeviceStatus는 장비 전체 상태입니다.
type DeviceStatus struct {
	Device     Device
	Up         bool
	UptimeSec  float64
	Interfaces []InterfaceStats
	CollectedAt time.Time
}

// ifCounter는 이전 폴링 값(delta 계산용)을 저장합니다.
type ifCounter struct {
	in, out uint64
	at      time.Time
}

// Poller는 SNMP 장비를 주기적으로 폴링합니다.
type Poller struct {
	mu       sync.RWMutex
	statuses map[int64]*DeviceStatus  // device ID → 최신 상태
	counters map[string]*ifCounter    // "deviceID:ifIndex" → 이전 카운터
}

func NewPoller() *Poller {
	return &Poller{
		statuses: make(map[int64]*DeviceStatus),
		counters: make(map[string]*ifCounter),
	}
}

// Poll은 단일 장비를 폴링하고 내부 상태를 갱신합니다.
func (p *Poller) Poll(dev Device) {
	status := p.poll(dev)
	p.mu.Lock()
	p.statuses[dev.ID] = status
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
	status := &DeviceStatus{Device: dev, CollectedAt: time.Now()}

	g := &gosnmp.GoSNMP{
		Target:    dev.IP,
		Port:      uint16(dev.Port),
		Community: dev.Community,
		Version:   gosnmp.Version2c,
		Timeout:   3 * time.Second,
		Retries:   1,
	}
	if err := g.Connect(); err != nil {
		log.Printf("[SNMP] %s(%s) 연결 실패: %v", dev.Name, dev.IP, err)
		return status
	}
	defer g.Conn.Close()

	// 시스템 기본 정보
	sysResult, err := g.Get([]string{oidSysName, oidSysUpTime})
	if err != nil {
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
