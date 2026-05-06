package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/db"
)

// LogHandler는 로그 수집/검색/라벨링 API를 처리합니다.
type LogHandler struct {
	store      *db.LogStore
	annStore   *db.LogAnnotationStore // 라벨링 (옵션 — nil이면 라벨 기능 비활성)
	agentStore *db.AgentStore         // 개별 키 검증용
	legacyKey  string                 // 하위 호환성용
}

func NewLogHandler(store *db.LogStore, annStore *db.LogAnnotationStore, agentStore *db.AgentStore, legacyKey string) *LogHandler {
	return &LogHandler{store: store, annStore: annStore, agentStore: agentStore, legacyKey: legacyKey}
}

// Ingest POST /api/logs/ingest — 에이전트에서 호출 (Agent Key 인증)
func (h *LogHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	// Agent Key 인증
	key := r.Header.Get("X-Agent-Key")
	
	valid := false
	if h.legacyKey != "" && key == h.legacyKey {
		valid = true
	} else if h.agentStore != nil {
		agent, err := h.agentStore.GetByKey(r.Context(), key)
		if err == nil && agent.Status == "active" {
			valid = true
			go h.agentStore.UpdateLastSeen(context.Background(), key)
		}
	}

	if !valid {
		http.Error(w, "인증 실패 또는 비활성화된 에이전트", http.StatusUnauthorized)
		return
	}

	var req struct {
		Entries []struct {
			Timestamp string `json:"timestamp"`
			Host      string `json:"host"`
			Source    string `json:"source"`
			FilePath  string `json:"file_path"`
			Line      string `json:"line"`
			Level     string `json:"level"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	records := make([]db.LogRecord, 0, len(req.Entries))
	for _, e := range req.Entries {
		ts, _ := time.Parse(time.RFC3339, e.Timestamp)
		if ts.IsZero() {
			ts = time.Now()
		}
		records = append(records, db.LogRecord{
			Timestamp: ts,
			Host:      e.Host,
			Source:    e.Source,
			FilePath:  e.FilePath,
			Line:      e.Line,
			Level:     e.Level,
		})
	}

	if err := h.store.Ingest(r.Context(), records); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// Search GET /api/logs — 로그 검색
func (h *LogHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := db.LogSearchParams{
		Host:   q.Get("host"),
		Source: q.Get("source"),
		Level:  q.Get("level"),
		Query:  q.Get("query"),
	}
	params.Limit, _ = strconv.Atoi(q.Get("limit"))
	if params.Limit <= 0 {
		params.Limit = 100
	}
	params.Offset, _ = strconv.Atoi(q.Get("offset"))

	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			params.From = t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			params.To = t
		}
	}

	result, err := h.store.Search(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Sources GET /api/logs/sources — 사용 가능한 로그 소스 목록
func (h *LogHandler) Sources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.Sources(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}

// GetByID GET /api/logs/{id} — 단일 로그 상세 조회 (라벨링 UI용)
func (h *LogHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	rec, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "로그를 찾을 수 없음", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}

// Annotate POST /api/logs/{id}/annotate — 로그에 (원인, 조치) 라벨 부여
//
// 요청 body:
//   { "category": "root_cause" | "action_taken" | "false_positive",
//     "problem": "...", "solution": "...", "alert_id": 123 }
//
// JWT 미들웨어에서 username을 추출해 annotator로 기록합니다.
func (h *LogHandler) Annotate(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}

	logID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}

	// 대상 로그 존재 확인 + log_timestamp 가져오기
	rec, err := h.store.GetByID(r.Context(), logID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "로그를 찾을 수 없음", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Category string `json:"category"`
		Problem  string `json:"problem"`
		Solution string `json:"solution"`
		AlertID  *int64 `json:"alert_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if req.Problem == "" && req.Solution == "" {
		http.Error(w, "problem 또는 solution 중 최소 하나는 필요", http.StatusBadRequest)
		return
	}

	annotator := auth.UsernameFromContext(r.Context())
	if annotator == "" {
		annotator = "anonymous"
	}

	a := &db.LogAnnotation{
		LogID:        logID,
		LogTimestamp: rec.Timestamp,
		Annotator:    annotator,
		Category:     req.Category,
		Problem:      req.Problem,
		Solution:     req.Solution,
		AlertID:      req.AlertID,
	}
	if err := h.annStore.Insert(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(a)
}

// ListAnnotationsByLog GET /api/logs/{id}/annotations — 특정 로그의 라벨 목록
func (h *LogHandler) ListAnnotationsByLog(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}
	logID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	list, err := h.annStore.ListByLogID(r.Context(), logID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.LogAnnotation{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// ListAnnotations GET /api/logs/annotations — 전체 라벨 목록 (학습 데이터 추출용)
func (h *LogHandler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	list, total, err := h.annStore.List(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.LogAnnotation{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"items": list,
		"total": total,
	})
}

// DeleteAnnotation DELETE /api/logs/annotations/{id} — 라벨 삭제 (오타 수정 등)
func (h *LogHandler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	if err := h.annStore.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
