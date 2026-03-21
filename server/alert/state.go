package alert

import (
	"sync"
	"time"
)

const cooldown = 30 * time.Minute // 같은 알림 최소 간격

// State는 알림 중복 방지 상태를 관리합니다.
type State struct {
	mu          sync.Mutex
	lastAlertAt map[string]time.Time // key: 알림 식별자
	firing      map[string]bool      // key: 현재 알림 발화 중인 항목
	failCounts  map[string]int       // key: 서비스 체크 연속 실패 카운트
	acked       map[string]bool      // key: "{host}/{category}/{severity}" 확인된 알림
	maintenance map[string]bool      // key: host — 유지보수 모드 호스트
}

func NewState() *State {
	return &State{
		lastAlertAt: make(map[string]time.Time),
		firing:      make(map[string]bool),
		failCounts:  make(map[string]int),
		acked:       make(map[string]bool),
		maintenance: make(map[string]bool),
	}
}

// ShouldAlert는 해당 알림을 보내야 하는지 확인합니다. (쿨다운 체크)
// 알림을 보내야 하면 firing 상태로 표시합니다.
func (s *State) ShouldAlert(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.lastAlertAt[key]
	if !ok || time.Since(last) >= cooldown {
		s.lastAlertAt[key] = time.Now()
		s.firing[key] = true
		return true
	}
	s.firing[key] = true
	return false
}

// ClearIfFiring은 해당 항목이 발화 중이었으면 상태를 초기화하고 true를 반환합니다.
// true 반환 시 회복 알림을 보내야 합니다.
func (s *State) ClearIfFiring(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing[key] {
		delete(s.firing, key)
		delete(s.lastAlertAt, key)
		return true
	}
	return false
}

// RecordFailure는 서비스 체크 연속 실패 횟수를 증가시키고 현재 횟수를 반환합니다.
func (s *State) RecordFailure(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCounts[key]++
	return s.failCounts[key]
}

// ResetFailure는 서비스 체크 연속 실패 횟수를 초기화합니다.
func (s *State) ResetFailure(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCounts[key] = 0
}

// --- 알림 Ack ---

func ackKey(host, category, severity string) string {
	return host + "/" + category + "/" + severity
}

// Ack는 알림을 확인 처리합니다. 이메일 재발송이 억제됩니다.
func (s *State) Ack(host, category, severity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acked[ackKey(host, category, severity)] = true
}

// IsAcked는 해당 알림이 확인됐는지 반환합니다.
func (s *State) IsAcked(host, category, severity string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acked[ackKey(host, category, severity)]
}

// ClearAck는 회복 시 ack를 해제합니다.
func (s *State) ClearAck(host, category, severity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.acked, ackKey(host, category, severity))
}

// --- 유지보수 모드 ---

// SetMaintenance는 호스트의 유지보수 모드를 설정/해제합니다.
func (s *State) SetMaintenance(host string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.maintenance[host] = true
	} else {
		delete(s.maintenance, host)
	}
}

// IsInMaintenance는 호스트가 유지보수 모드인지 반환합니다.
func (s *State) IsInMaintenance(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maintenance[host]
}

// MaintenanceHosts는 유지보수 중인 호스트 목록을 반환합니다.
func (s *State) MaintenanceHosts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts := make([]string, 0, len(s.maintenance))
	for h := range s.maintenance {
		hosts = append(hosts, h)
	}
	return hosts
}
