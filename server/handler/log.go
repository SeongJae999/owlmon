package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/seongJae/owlmon/server/db"
)

// LogHandler는 로그 수집/검색 API를 처리합니다.
type LogHandler struct {
	store      *db.LogStore
	agentStore *db.AgentStore // 개별 키 검증용
	legacyKey  string         // 하위 호환성용
}

func NewLogHandler(store *db.LogStore, agentStore *db.AgentStore, legacyKey string) *LogHandler {
	return &LogHandler{store: store, agentStore: agentStore, legacyKey: legacyKey}
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
