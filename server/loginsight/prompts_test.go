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

// TestSystemPromptLogCluster_FewShot — few-shot 판단 예시가 살아있는지 검증.
// 정상종료(low)·진짜장애(high/critical)·애매한 1회성(medium) 예시가
// 모두 있어야 경계 케이스 판단이 일관된다.
func TestSystemPromptLogCluster_FewShot(t *testing.T) {
	required := []string{
		"판단 예시", // few-shot 섹션 존재
		"[입력]",   // 입력 예시 마커
		"[출력]",   // 정답 예시 마커
		"pmlogger", // 정상종료 예시 (low)
		"mysqld",   // 진짜 장애 예시 (high)
	}
	for _, kw := range required {
		if !strings.Contains(SystemPromptLogCluster, kw) {
			t.Errorf("SystemPromptLogCluster에 few-shot 키워드 %q 누락", kw)
		}
	}
	if n := strings.Count(SystemPromptLogCluster, "[입력]"); n < 5 {
		t.Errorf("few-shot 예시가 %d개 — 최소 5개 기대 (정상·장애·애매 케이스 커버)", n)
	}
	if strings.Count(SystemPromptLogCluster, "[입력]") != strings.Count(SystemPromptLogCluster, "[출력]") {
		t.Error("few-shot 입력/출력 마커 개수 불일치 — 예시 쌍이 깨졌을 수 있음")
	}
}
