package loginsight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/llm"
)

// MaskFunc — Analyzer에 주입할 PII 마스킹 함수 시그니처.
// nil이면 마스킹 없이 원문 전송 (망내 Ollama 등).
type MaskFunc func(string) string

// Analyzer — LogGroup을 LLM에 보내고 검증된 Insight를 반환한다.
//
// 의존성 격리:
//   - llm.Provider만 외부 의존 (이미 server/llm/에 추상화됨)
//   - 마스킹은 옵션 주입 (server/masking 직접 의존 없음)
type Analyzer struct {
	provider llm.Provider
	maskFn   MaskFunc
}

// Option — 함수형 옵션 패턴.
type Option func(*Analyzer)

// WithMasking — 외부 LLM(OpenAI 등) 호출 전에 적용할 마스킹 함수.
//
//	a := NewAnalyzer(provider, WithMasking(func(s string) string {
//	    return masking.Mask(s, masking.DefaultOptions())
//	}))
func WithMasking(fn MaskFunc) Option {
	return func(a *Analyzer) { a.maskFn = fn }
}

// NewAnalyzer — provider가 nil이면 nil 반환 (LLM 비활성 환경 대응).
func NewAnalyzer(provider llm.Provider, opts ...Option) *Analyzer {
	if provider == nil {
		return nil
	}
	a := &Analyzer{provider: provider}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Analyze — LogGroup 한 개를 LLM으로 분석하고 검증된 Insight 반환.
// ID는 0 (DB 저장 시 채워짐). 메타데이터(model_name, latency_ms, created_at,
// template_hash, host_name, sample_log_ids)는 자동으로 채워진다.
func (a *Analyzer) Analyze(ctx context.Context, g LogGroup) (*Insight, error) {
	if a == nil || a.provider == nil {
		return nil, errors.New("analyzer: provider 미설정")
	}

	userMsg := BuildUserMsg(g)
	if a.maskFn != nil {
		userMsg = a.maskFn(userMsg)
	}

	start := time.Now()
	raw, err := a.provider.Complete(ctx, SystemPromptLogCluster, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM 호출 실패: %w", err)
	}
	latency := time.Since(start)

	cleaned := extractJSON(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("LLM 응답에서 JSON을 찾을 수 없습니다: %s",
			truncate(raw, 200))
	}

	var ins Insight
	if err := json.Unmarshal([]byte(cleaned), &ins); err != nil {
		return nil, fmt.Errorf("JSON 파싱 실패: %w (원문: %s)",
			err, truncate(cleaned, 200))
	}
	if err := ins.Validate(); err != nil {
		return nil, fmt.Errorf("Insight 검증 실패: %w", err)
	}

	// 메타데이터 — DB 저장 직전 호출자가 추가로 덮어쓸 수 있음.
	ins.TemplateHash = g.TemplateHash
	ins.HostName = g.HostName
	ins.SampleLogIDs = g.SampleLogIDs
	ins.ModelName = a.provider.Name()
	ins.LatencyMs = int(latency.Milliseconds())
	ins.CreatedAt = time.Now()

	return &ins, nil
}

// extractJSON — LLM이 코드펜스(```json) / 설명문을 섞어 보내도
// 가장 바깥쪽 { ... } 한 쌍만 잘라낸다.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
