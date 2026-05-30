package loginsight

import (
	"errors"
	"time"
)

// Severity — LLM 출력 severity enum.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// Category — LLM 출력 category enum.
type Category string

const (
	CatAuth     Category = "auth"
	CatNetwork  Category = "network"
	CatDisk     Category = "disk"
	CatDB       Category = "db"
	CatApp      Category = "app"
	CatSecurity Category = "security"
	CatOther    Category = "other"
)

// Insight — LLM 분석 결과 1건. log_insights 테이블의 1행에 대응한다.
type Insight struct {
	ID           int64   `json:"id"`
	TemplateHash string  `json:"template_hash"`
	SampleLogIDs []int64 `json:"sample_log_ids"`
	HostName     string  `json:"host_name,omitempty"`

	// LLM이 JSON으로 채우는 필드
	Severity    Severity `json:"severity"`
	Category    Category `json:"category"`
	SummaryKo   string   `json:"summary_ko"`
	RootCauseKo string   `json:"root_cause_ko,omitempty"`
	ActionKo    string   `json:"action_ko,omitempty"`
	NeedsHuman  bool     `json:"needs_human"`

	// 메타데이터
	ModelName    string    `json:"model_name"`
	PromptTokens int       `json:"prompt_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	LatencyMs    int       `json:"latency_ms,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Validate — LLM 응답 파싱 직후 호출. enum/길이 검증.
func (i *Insight) Validate() error {
	if i.SummaryKo == "" {
		return errors.New("summary_ko는 비어있을 수 없습니다")
	}
	if len(i.SummaryKo) > 200 {
		return errors.New("summary_ko가 너무 깁니다 (최대 200자)")
	}
	switch i.Severity {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
	default:
		return errors.New("severity 값이 유효하지 않습니다: " + string(i.Severity))
	}
	switch i.Category {
	case CatAuth, CatNetwork, CatDisk, CatDB, CatApp, CatSecurity, CatOther:
	default:
		return errors.New("category 값이 유효하지 않습니다: " + string(i.Category))
	}
	return nil
}

// LogGroup — 워커가 만들어 analyzer에 전달하는 클러스터링된 로그 그룹.
type LogGroup struct {
	TemplateHash    string   // 16자 해시 (Drain-lite)
	TemplateExample string   // 정규화된 템플릿 ("<IP>", "<N>" 치환)
	HostName        string   // 그룹의 대표 호스트
	Count           int      // 5분 윈도우 내 발생 수
	SampleLines     []string // 원문 샘플 (최대 3건)
	SampleLogIDs    []int64  // logs 테이블 PK 배열
}
