//go:build !linux

package logtail

import (
	"context"
	"log"
)

// JournaldCollector는 Linux 전용 — 다른 OS에서는 no-op stub.
//
// Linux journald_linux.go 와 같은 인터페이스를 제공하되 실제 동작은 안 함.
// main.go가 OS 분기 없이 통일된 코드로 collector를 참조할 수 있게 함.
type JournaldCollector struct{}

// NewJournaldCollector는 stub 인스턴스를 반환합니다.
// 인자는 호환성을 위해 받지만 사용되지 않습니다.
func NewJournaldCollector(_ string, _ string, _ func(LogEntry)) *JournaldCollector {
	return &JournaldCollector{}
}

// Start는 한 번 경고 로그만 남기고 즉시 반환합니다.
func (c *JournaldCollector) Start(_ context.Context) {
	log.Println("[journald] 이 OS에서는 systemd journal 미지원 — 비활성")
}
