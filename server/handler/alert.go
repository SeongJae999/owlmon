package handler

import (
	"encoding/json"
	"net/http"

	"github.com/seongJae/owlmon/server/alert"
)

type AlertHandler struct {
	store alert.ConfigStorer
}

func NewAlertHandler(store alert.ConfigStorer) *AlertHandler {
	return &AlertHandler{store: store}
}

// GetConfig는 현재 알림 설정을 반환합니다.
func (h *AlertHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.store.Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// SetConfig는 알림 설정을 업데이트합니다.
func (h *AlertHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	var cfg alert.AlertConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	if err := h.store.Set(cfg); err != nil {
		http.Error(w, "설정 저장 실패", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}
