package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/synthetic"
)

// SyntheticHandler는 Synthetic 모니터링 API를 처리합니다.
type SyntheticHandler struct {
	store   *db.SyntheticStore
	checker *synthetic.Checker
}

func NewSyntheticHandler(store *db.SyntheticStore, checker *synthetic.Checker) *SyntheticHandler {
	return &SyntheticHandler{store: store, checker: checker}
}

// ListMonitors GET /api/synthetic/monitors
func (h *SyntheticHandler) ListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if monitors == nil {
		monitors = []synthetic.Monitor{}
	}
	writeJSON(w, monitors)
}

// AddMonitor POST /api/synthetic/monitors
func (h *SyntheticHandler) AddMonitor(w http.ResponseWriter, r *http.Request) {
	var m synthetic.Monitor
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if msg := validateMonitor(&m); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	created, err := h.store.Add(context.Background(), m)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 백그라운드 즉시 1회 체크 + 루프 동기화
	go func() {
		h.checker.TriggerOne(created)
	}()

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

// UpdateMonitor PUT /api/synthetic/monitors/{id}
func (h *SyntheticHandler) UpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	var m synthetic.Monitor
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	m.ID = id
	if msg := validateMonitor(&m); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if err := h.store.Update(context.Background(), m); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteMonitor DELETE /api/synthetic/monitors/{id}
func (h *SyntheticHandler) DeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	if err := h.store.Delete(context.Background(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SyntheticStatusItem은 모니터 + 최근 결과 + 통계를 합친 응답입니다.
type SyntheticStatusItem struct {
	Monitor synthetic.Monitor   `json:"monitor"`
	Latest  *synthetic.Result   `json:"latest,omitempty"`
	Stats   db.SyntheticStats   `json:"stats"`
}

// GetStatus GET /api/synthetic/status — 모든 모니터의 최근 상태 + 24h 통계
func (h *SyntheticHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	monitors, err := h.store.List(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	latest := h.checker.GetLatest()

	items := make([]SyntheticStatusItem, 0, len(monitors))
	for _, m := range monitors {
		stats, _ := h.store.Stats(ctx, m.ID, 24)
		item := SyntheticStatusItem{Monitor: m, Stats: stats}
		if r, ok := latest[m.ID]; ok {
			rc := r
			item.Latest = &rc
		}
		items = append(items, item)
	}
	writeJSON(w, items)
}

// GetHistory GET /api/synthetic/monitors/{id}/history?limit=100
func (h *SyntheticHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	limit := 100
	if l := queryInt(r.URL.Query(), "limit", 100); l > 0 && l <= 1000 {
		limit = l
	}
	results, err := h.store.RecentResults(context.Background(), id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []synthetic.Result{}
	}
	writeJSON(w, results)
}

// TriggerCheck POST /api/synthetic/monitors/{id}/check — 즉시 체크
func (h *SyntheticHandler) TriggerCheck(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	monitors, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, m := range monitors {
		if m.ID == id {
			result := h.checker.TriggerOne(m)
			writeJSON(w, result)
			return
		}
	}
	http.Error(w, "모니터를 찾을 수 없음", http.StatusNotFound)
}

func validateMonitor(m *synthetic.Monitor) string {
	if strings.TrimSpace(m.Name) == "" {
		return "이름은 필수입니다"
	}
	if strings.TrimSpace(m.URL) == "" {
		return "URL은 필수입니다"
	}
	u, err := url.Parse(m.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "URL은 http:// 또는 https://로 시작해야 합니다"
	}
	if m.Method == "" {
		m.Method = "GET"
	}
	if m.ExpectedStatus == 0 {
		m.ExpectedStatus = 200
	}
	if m.IntervalSeconds == 0 {
		m.IntervalSeconds = 60
	}
	if m.IntervalSeconds < 10 {
		return "최소 체크 주기는 10초입니다"
	}
	if m.TimeoutSeconds == 0 {
		m.TimeoutSeconds = 10
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func queryInt(q url.Values, key string, def int) int {
	if v := q.Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
