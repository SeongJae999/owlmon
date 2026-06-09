package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/seongJae/owlmon/server/loginsight"
)

// insightStore — Handler가 사용하는 store 인터페이스. 테스트를 위해 분리.
// 실제로는 *loginsight.Store가 만족.
type insightStore interface {
	List(ctx context.Context, f loginsight.Filter) ([]loginsight.Insight, error)
	ByTemplate(ctx context.Context, hash string, maxAge time.Duration) (*loginsight.Insight, error)
}

// InsightHandler — log_insights 조회 API.
type InsightHandler struct {
	store insightStore
}

// NewInsightHandler — store가 nil이면 모든 엔드포인트가 503 반환.
func NewInsightHandler(store insightStore) *InsightHandler {
	return &InsightHandler{store: store}
}

// Enabled — 워커/스토어 활성 여부.
func (h *InsightHandler) Enabled() bool {
	return h != nil && h.store != nil
}

// Status — GET /api/insights/status
func (h *InsightHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSONInsight(w, map[string]any{
		"enabled": h.Enabled(),
	})
}

// List — GET /api/insights/list
// Query: host, severity, from (RFC3339), to (RFC3339), limit
func (h *InsightHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		http.Error(w, "log-insight 기능이 비활성화되어 있습니다", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	f := loginsight.Filter{
		HostName: q.Get("host"),
		Severity: q.Get("severity"),
	}
	if s := q.Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.From = t
		}
	}
	if s := q.Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.To = t
		}
	}
	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			f.Limit = n
		}
	}

	items, err := h.store.List(r.Context(), f)
	if err != nil {
		http.Error(w, "조회 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []loginsight.Insight{}
	}
	writeJSONInsight(w, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// ByTemplate — GET /api/insights/by-template?hash=X
// 캐시된 인사이트 단일 조회 (24h TTL).
func (h *InsightHandler) ByTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		http.Error(w, "log-insight 기능이 비활성화되어 있습니다", http.StatusServiceUnavailable)
		return
	}

	hash := r.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "hash 쿼리 파라미터가 필요합니다", http.StatusBadRequest)
		return
	}

	ins, err := h.store.ByTemplate(r.Context(), hash, 24*time.Hour)
	if err != nil {
		http.Error(w, "조회 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ins == nil {
		http.Error(w, "해당 템플릿의 인사이트가 없습니다", http.StatusNotFound)
		return
	}
	writeJSONInsight(w, ins)
}

func writeJSONInsight(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
