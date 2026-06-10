package rules

import (
	"regexp"
	"testing"
)

// 테스트용 엔진 — DB 없이 컴파일된 룰을 직접 주입한다.
func testEngine(rules ...Rule) *Engine {
	e := &Engine{}
	e.rules = rules
	return e
}

func mustRule(id int64, name, pattern, severity string) Rule {
	return Rule{
		ID:              id,
		Name:            name,
		Severity:        severity,
		ThresholdCount:  0,
		ThresholdWindow: 60,
		CooldownSeconds: 300,
		compiled:        regexp.MustCompile(pattern),
	}
}

// 패턴이 맞는 라인만 매칭된다.
func TestEvaluate_BasicMatch(t *testing.T) {
	e := testEngine(mustRule(1, "OOM 감지", `(?i)out of memory`, "critical"))

	if got := e.Evaluate("kernel: Out of memory: Kill process 1234"); len(got) != 1 {
		t.Fatalf("OOM 라인은 1건 매칭이어야 하는데 %d건", len(got))
	}
	if got := e.Evaluate("정상 동작 로그입니다"); len(got) != 0 {
		t.Fatalf("무관한 라인은 0건이어야 하는데 %d건", len(got))
	}
}

// 한 라인에 여러 룰이 동시에 매칭될 수 있다.
func TestEvaluate_MultipleRules(t *testing.T) {
	e := testEngine(
		mustRule(1, "에러", `(?i)error`, "warning"),
		mustRule(2, "디스크", `(?i)disk`, "critical"),
		mustRule(3, "무관", `never-match-xyz`, "info"),
	)

	got := e.Evaluate("ERROR: disk read failure")
	if len(got) != 2 {
		t.Fatalf("2건 매칭이어야 하는데 %d건", len(got))
	}
}

// 매칭 결과가 룰의 알림 파라미터(임계치·쿨다운)를 그대로 전달한다 —
// 핸들러가 이 값으로 알림 발사를 결정하므로 누락되면 안 된다.
func TestEvaluate_CarriesRuleFields(t *testing.T) {
	r := mustRule(7, "로그인 실패", `Failed password`, "warning")
	r.ThresholdCount = 5
	r.ThresholdWindow = 120
	r.CooldownSeconds = 600
	e := testEngine(r)

	got := e.Evaluate("sshd: Failed password for root")
	if len(got) != 1 {
		t.Fatalf("1건 매칭 기대, %d건", len(got))
	}
	m := got[0]
	if m.RuleID != 7 || m.Severity != "warning" ||
		m.ThresholdCount != 5 || m.ThresholdWindow != 120 || m.CooldownSeconds != 600 {
		t.Fatalf("룰 파라미터가 그대로 전달돼야 함: %+v", m)
	}
}

// 룰이 없으면 어떤 라인도 매칭되지 않는다 (panic 없이).
func TestEvaluate_EmptyRules(t *testing.T) {
	e := testEngine()
	if got := e.Evaluate("anything"); len(got) != 0 {
		t.Fatalf("룰 없으면 0건이어야 하는데 %d건", len(got))
	}
}

func TestRuleCount(t *testing.T) {
	e := testEngine(
		mustRule(1, "a", `a`, "info"),
		mustRule(2, "b", `b`, "info"),
	)
	if got := e.RuleCount(); got != 2 {
		t.Fatalf("RuleCount=2 기대, %d", got)
	}
}
