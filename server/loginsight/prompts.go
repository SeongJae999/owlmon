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
- 출력은 한국어 (필드 키는 영문 그대로).`

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
