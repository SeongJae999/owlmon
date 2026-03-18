package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/seongJae/owlmon/server/db"
)

type HistoryHandler struct {
	store *db.AlertHistoryStore
}

func NewHistoryHandler(store *db.AlertHistoryStore) *HistoryHandler {
	return &HistoryHandler{store: store}
}

// List는 최근 알림 히스토리를 반환합니다.
func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	records, err := h.store.List(r.Context(), limit)
	if err != nil {
		http.Error(w, "히스토리 조회 실패", http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []db.AlertRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}
