package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/masking"
)

// LLMHandler — 로그 설명/알림 요약 등 LLM 기반 API.
type LLMHandler struct {
	provider  llm.Provider
	cache     *llm.Cache
	historyStore *db.AlertHistoryStore
}

func NewLLMHandler(provider llm.Provider, historyStore *db.AlertHistoryStore) *LLMHandler {
	return &LLMHandler{
		provider:     provider,
		cache:        llm.NewCache(500, 24*time.Hour),
		historyStore: historyStore,
	}
}

// Enabled — LLM 기능 활성 여부 (provider 존재 여부).
func (h *LLMHandler) Enabled() bool {
	return h.provider != nil
}

// Status GET /api/llm/status — LLM 활성/비활성 + provider 이름
func (h *LLMHandler) Status(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"enabled": h.Enabled(),
	}
	if h.provider != nil {
		resp["provider"] = h.provider.Name()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ExplainLog POST /api/llm/explain — 로그 한 줄을 한국어로 설명.
// Body: {"line":"..."}
// Response: {"explanation":"...","cached":bool,"masked":bool}
func (h *LLMHandler) ExplainLog(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		http.Error(w, "LLM 기능이 비활성화되어 있습니다 (OWLMON_LLM_PROVIDER 미설정)", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Line string `json:"line"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Line) == "" {
		http.Error(w, "line은 비어있을 수 없습니다", http.StatusBadRequest)
		return
	}
	if len(req.Line) > 2000 {
		req.Line = req.Line[:2000] + "...(생략)"
	}

	// 외부 LLM 호출 전 PII 마스킹 (이메일/IP/계정명 등)
	masked := masking.Mask(req.Line, masking.DefaultOptions())
	wasMasked := masked != req.Line

	cacheKey := hash("explain:" + h.provider.Name() + ":" + masked)
	if v, ok := h.cache.Get(cacheKey); ok {
		writeJSONLLM(w, map[string]interface{}{
			"explanation": v,
			"cached":      true,
			"masked":      wasMasked,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	text, err := h.provider.Complete(ctx, llm.SystemPromptLogExplain, masked)
	if err != nil {
		http.Error(w, "LLM 호출 실패: "+err.Error(), http.StatusBadGateway)
		return
	}
	h.cache.Set(cacheKey, text)

	writeJSONLLM(w, map[string]interface{}{
		"explanation": text,
		"cached":      false,
		"masked":      wasMasked,
	})
}

// SummarizeAlerts POST /api/llm/summary — 지난 24시간(또는 지정 기간) 알림 요약.
// Body: {"hours": 24} (옵션, 기본 24)
// Response: {"summary":"...","total":N,"cached":bool}
func (h *LLMHandler) SummarizeAlerts(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		http.Error(w, "LLM 기능이 비활성화되어 있습니다", http.StatusServiceUnavailable)
		return
	}
	if h.historyStore == nil {
		http.Error(w, "알림 히스토리 저장소가 비활성화되어 있습니다", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Hours int `json:"hours"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Hours <= 0 || req.Hours > 168 {
		req.Hours = 24
	}

	now := time.Now()
	from := now.Add(-time.Duration(req.Hours) * time.Hour)
	records, err := h.historyStore.Search(r.Context(), db.AlertHistoryFilter{
		From:  from,
		To:    now,
		Limit: 500,
	})
	if err != nil {
		http.Error(w, "알림 히스토리 조회 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(records) == 0 {
		writeJSONLLM(w, map[string]interface{}{
			"summary": "지난 " + fmt.Sprintf("%d", req.Hours) + "시간 동안 발송된 알림이 없습니다. 시스템 안정 상태입니다.",
			"total":   0,
			"cached":  false,
		})
		return
	}

	// LLM 입력용 텍스트 변환 — host/severity/category 등 압축
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("기간: 최근 %d시간\n총 %d건\n\n", req.Hours, len(records)))

	// 패턴 분석을 돕기 위해 간단한 통계 함께 전달
	bySeverity := map[string]int{}
	byHost := map[string]int{}
	byCategory := map[string]int{}
	for _, rec := range records {
		bySeverity[rec.Severity]++
		byHost[rec.Host]++
		byCategory[rec.Category]++
	}
	sb.WriteString("심각도별: ")
	for k, v := range bySeverity {
		sb.WriteString(fmt.Sprintf("%s=%d ", k, v))
	}
	sb.WriteString("\n호스트별: ")
	for k, v := range byHost {
		sb.WriteString(fmt.Sprintf("%s=%d ", k, v))
	}
	sb.WriteString("\n카테고리별: ")
	for k, v := range byCategory {
		sb.WriteString(fmt.Sprintf("%s=%d ", k, v))
	}
	sb.WriteString("\n\n---최근 알림 50개 (시간/심각도/호스트/제목)---\n")
	limit := len(records)
	if limit > 50 {
		limit = 50
	}
	for i := 0; i < limit; i++ {
		rec := records[i]
		sb.WriteString(fmt.Sprintf("%s | %s | %s | %s\n",
			rec.SentAt.Format("15:04"), rec.Severity, rec.Host, rec.Subject))
	}

	userPrompt := sb.String()
	cacheKey := hash("summary:" + h.provider.Name() + ":" + userPrompt)
	if v, ok := h.cache.Get(cacheKey); ok {
		writeJSONLLM(w, map[string]interface{}{
			"summary": v,
			"total":   len(records),
			"cached":  true,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	text, err := h.provider.Complete(ctx, llm.SystemPromptAlertSummary, userPrompt)
	if err != nil {
		http.Error(w, "LLM 호출 실패: "+err.Error(), http.StatusBadGateway)
		return
	}
	h.cache.Set(cacheKey, text)

	writeJSONLLM(w, map[string]interface{}{
		"summary": text,
		"total":   len(records),
		"cached":  false,
	})
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:16])
}

func writeJSONLLM(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
