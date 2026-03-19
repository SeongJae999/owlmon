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
}

func NewState() *State {
	return &State{
		lastAlertAt: make(map[string]time.Time),
		firing:      make(map[string]bool),
		failCounts:  make(map[string]int),
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
