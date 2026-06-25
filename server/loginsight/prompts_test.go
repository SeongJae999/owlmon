package loginsight

import (
	"strings"
	"testing"
)

// TestSystemPromptLogCluster_NormalExitGuard — 정상 종료 오탐 방지 지침이
// 프롬프트에 살아있는지 검증 (실수로 삭제/변경되는 회귀 방지).
// pmlogger_daily 정상 종료를 "실패"로 오탐한 사례(2026-06) 재발 방지용.
func TestSystemPromptLogCluster_NormalExitGuard(t *testing.T) {
	required := []string{
		"정상 종료",   // 오탐 방지 섹션 존재
		"oneshot",     // 배치/oneshot 개념
		"Succeeded",   // systemd 성공 신호
		"logrotate",   // 대표 배치 서비스 예시
		"non-zero",    // 비정상 신호일 때만 격상
	}
	for _, kw := range required {
		if !strings.Contains(SystemPromptLogCluster, kw) {
			t.Errorf("SystemPromptLogCluster에 정상종료 가드 키워드 %q 누락 — 오탐 방지 지침이 사라졌을 수 있음", kw)
		}
	}
}
