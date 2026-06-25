package loginsight

import (
	"strconv"
	"strings"
)

// SystemPromptLogCluster — 클러스터링된 로그 그룹을 JSON으로 분석하라는 시스템 프롬프트.
//
// 기존 server/llm/prompts.go (평문 출력 explain/summary)와 별개로 둔다.
// JSON 출력 강제 + 운영자(IT 비전문가) 대상 한국어 + 학교/공공기관 톤.
//
// 향후 server/llm/prompts.go 통합 검토 가능 (지금은 패키지별 분리 유지).
const SystemPromptLogCluster = `당신은 학교/공공기관 IT 시스템 모니터링 전문가입니다.
주어진 영문 로그 그룹(같은 패턴으로 반복 발생)을 분석하여 **반드시 아래 JSON 한 개만** 출력하세요.

스키마:
{
  "severity":      "critical" | "high" | "medium" | "low",
  "category":      "auth" | "network" | "disk" | "db" | "app" | "security" | "other",
  "summary_ko":    "한 문장 한국어 요약 (60자 이내)",
  "root_cause_ko": "추정 원인 (한국어, 100자 이내)",
  "action_ko":     "권장 조치 (한국어, 명령어/경로 포함 가능)",
  "needs_human":   true | false
}

제약:
- 마크다운/설명문/코드펜스 금지. JSON 객체 하나만 출력.
- 정상 로그면 severity="low", needs_human=false.
- 추측이 강하면 root_cause_ko에 "추정" 명시.
- 학교 IT 담당자(비전문가)도 따라할 수 있게 action_ko 작성.
- 출력은 한국어 (필드 키는 영문 그대로).

정상 종료 오탐 금지 (매우 중요):
- 배치/oneshot 서비스(이름에 daily, weekly, rotate, logrotate, cleanup, backup,
  renew, certbot, pmlogger, mandb, updatedb, fstrim 등 포함)가 작업을 마치고
  종료되는 것은 정상이다.
- 다음 신호가 있으면 "실패"가 아니라 "정상 완료"로 판단하라 → severity="low",
  needs_human=false, summary_ko는 "정상 완료"임을 명시:
    · status=0/SUCCESS, code=exited status=0
    · "Succeeded", "Deactivated successfully", "Finished"
    · systemd가 oneshot/배치 유닛을 종료(Stopped/inactive)시키는 일상 메시지
- "종료됨(stopped/exited/deactivated)" 자체는 실패가 아니다. 종료 코드와 성공
  메시지를 먼저 확인하고, 비정상 신호(non-zero exit, failed, error, core dump,
  segfault, OOM)가 있을 때만 medium 이상으로 올려라.
- 확신이 없으면 과장하지 말고 root_cause_ko에 "정상 동작일 가능성 — 추가 확인 필요"로 적어라.`

// BuildUserMsg — LogGroup을 LLM 유저 메시지로 변환.
// 워커가 Provider.Complete(SystemPromptLogCluster, BuildUserMsg(group)) 호출에 사용.
func BuildUserMsg(g LogGroup) string {
	var b strings.Builder
	b.WriteString("Host: ")
	b.WriteString(g.HostName)
	b.WriteByte('\n')
	b.WriteString("Pattern (occurred ")
	b.WriteString(strconv.Itoa(g.Count))
	b.WriteString(" times in last 5 min):\n")
	b.WriteString(g.TemplateExample)
	b.WriteString("\n\nSample log lines:\n")
	for i, line := range g.SampleLines {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("] ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
