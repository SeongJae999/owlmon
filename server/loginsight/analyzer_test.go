package loginsight

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/seongJae/owlmon/server/llm"
)

// stubProvider — 고정 응답을 반환하는 테스트용 Provider.
type stubProvider struct {
	name     string
	response string
	err      error
	lastSys  string
	lastUser string
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Complete(ctx context.Context, sys, user string) (string, error) {
	s.lastSys = sys
	s.lastUser = user
	if s.err != nil {
		return "", s.err
	}
	return s.response, nil
}

func sampleGroup() LogGroup {
	return LogGroup{
		TemplateHash:    "abc12345",
		TemplateExample: "connection refused <IP>",
		HostName:        "web1",
		Count:           5,
		SampleLines:     []string{"line a", "line b"},
		SampleLogIDs:    []int64{10, 11},
	}
}

func validJSON() string {
	return `{"severity":"low","category":"app","summary_ko":"정상","needs_human":false}`
}

func TestAnalyzer_NilProvider(t *testing.T) {
	if NewAnalyzer(nil) != nil {
		t.Error("nil provider should return nil analyzer")
	}
}

func TestAnalyzer_SuccessJSON(t *testing.T) {
	stub := &stubProvider{
		name: "ollama:qwen2.5:7b",
		response: `{
			"severity": "high",
			"category": "network",
			"summary_ko": "연결이 거부되었습니다",
			"root_cause_ko": "방화벽 차단 추정",
			"action_ko": "방화벽 규칙 확인",
			"needs_human": true
		}`,
	}
	a := NewAnalyzer(stub)
	ins, err := a.Analyze(context.Background(), sampleGroup())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ins.Severity != SeverityHigh {
		t.Errorf("severity=%q want high", ins.Severity)
	}
	if ins.Category != CatNetwork {
		t.Errorf("category=%q want network", ins.Category)
	}
	if ins.SummaryKo != "연결이 거부되었습니다" {
		t.Errorf("summary_ko mismatch: %q", ins.SummaryKo)
	}
	if !ins.NeedsHuman {
		t.Error("needs_human should be true")
	}
	if ins.ModelName != "ollama:qwen2.5:7b" {
		t.Errorf("model_name=%q", ins.ModelName)
	}
	if ins.HostName != "web1" || ins.TemplateHash != "abc12345" {
		t.Errorf("metadata not populated: host=%q hash=%q", ins.HostName, ins.TemplateHash)
	}
	if len(ins.SampleLogIDs) != 2 {
		t.Errorf("sample_log_ids len=%d", len(ins.SampleLogIDs))
	}
	if ins.LatencyMs < 0 {
		t.Errorf("latency_ms should be >=0, got %d", ins.LatencyMs)
	}
	if ins.CreatedAt.IsZero() {
		t.Error("created_at should be populated")
	}
}

func TestAnalyzer_HandlesMarkdownFence(t *testing.T) {
	stub := &stubProvider{
		name:     "ollama:qwen2.5:7b",
		response: "```json\n" + validJSON() + "\n```",
	}
	a := NewAnalyzer(stub)
	ins, err := a.Analyze(context.Background(), sampleGroup())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ins.SummaryKo != "정상" {
		t.Errorf("summary_ko=%q", ins.SummaryKo)
	}
}

func TestAnalyzer_HandlesPrefixText(t *testing.T) {
	// LLM이 JSON 앞뒤로 설명 문장을 붙여도 추출 성공.
	stub := &stubProvider{
		name:     "ollama:qwen2.5:7b",
		response: "Here is the analysis:\n" + validJSON() + "\nThank you.",
	}
	a := NewAnalyzer(stub)
	ins, err := a.Analyze(context.Background(), sampleGroup())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ins.Category != CatApp {
		t.Errorf("category=%q", ins.Category)
	}
}

func TestAnalyzer_InvalidJSON(t *testing.T) {
	stub := &stubProvider{
		name:     "ollama:qwen2.5:7b",
		response: "this is not json at all",
	}
	a := NewAnalyzer(stub)
	if _, err := a.Analyze(context.Background(), sampleGroup()); err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestAnalyzer_InvalidSeverity(t *testing.T) {
	stub := &stubProvider{
		name:     "ollama:qwen2.5:7b",
		response: `{"severity":"unknown","category":"app","summary_ko":"ok","needs_human":false}`,
	}
	a := NewAnalyzer(stub)
	_, err := a.Analyze(context.Background(), sampleGroup())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "severity") {
		t.Errorf("error should mention severity, got: %v", err)
	}
}

func TestAnalyzer_ProviderError(t *testing.T) {
	stub := &stubProvider{
		name: "ollama:qwen2.5:7b",
		err:  errors.New("network timeout"),
	}
	a := NewAnalyzer(stub)
	if _, err := a.Analyze(context.Background(), sampleGroup()); err == nil {
		t.Error("expected error when provider fails")
	}
}

func TestAnalyzer_WithMasking(t *testing.T) {
	stub := &stubProvider{name: "openai:gpt-4o-mini", response: validJSON()}
	a := NewAnalyzer(stub, WithMasking(func(s string) string {
		return strings.ReplaceAll(s, "192.168.0.10", "<IP_MASKED>")
	}))
	g := sampleGroup()
	g.SampleLines = []string{"connection from 192.168.0.10 refused"}
	if _, err := a.Analyze(context.Background(), g); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if strings.Contains(stub.lastUser, "192.168.0.10") {
		t.Errorf("IP should be masked before sending, got user msg: %s", stub.lastUser)
	}
	if !strings.Contains(stub.lastUser, "<IP_MASKED>") {
		t.Errorf("masked token missing: %s", stub.lastUser)
	}
}

func TestAnalyzer_WithoutMasking(t *testing.T) {
	stub := &stubProvider{name: "ollama:qwen2.5:7b", response: validJSON()}
	a := NewAnalyzer(stub) // 마스킹 옵션 없음
	g := sampleGroup()
	g.SampleLines = []string{"connection from 192.168.0.10 refused"}
	if _, err := a.Analyze(context.Background(), g); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !strings.Contains(stub.lastUser, "192.168.0.10") {
		t.Error("IP should be preserved without masking (망내 Ollama)")
	}
}

func TestAnalyzer_UsesMockProviderGracefully(t *testing.T) {
	// llm.MockProvider는 평문을 뱉으므로 JSON 파싱 실패 — panic 없이 error만.
	a := NewAnalyzer(&llm.MockProvider{})
	if _, err := a.Analyze(context.Background(), sampleGroup()); err == nil {
		t.Error("MockProvider should fail JSON parsing (expected)")
	}
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"json fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"bare fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"surrounding text", `Some text {"a":1} more text`, `{"a":1}`},
		{"nested", `{"a":{"b":1}}`, `{"a":{"b":1}}`},
		{"no json", "no json here", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractJSON(c.in)
			if got != c.want {
				t.Errorf("extractJSON(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
