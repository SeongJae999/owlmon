package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
)

type AgentHandler struct {
	store *db.AgentStore
}

func NewAgentHandler(store *db.AgentStore) *AgentHandler {
	return &AgentHandler{store: store}
}

// Register POST /api/agent/register — 에이전트 신규 등록 요청
func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}

	// 새로운 키 생성
	b := make([]byte, 16)
	rand.Read(b)
	key := hex.EncodeToString(b)

	if err := h.store.Upsert(context.Background(), req.Name, key); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"key":    key,
		"status": "pending",
	})
}

// List GET /api/agents — 관리자용 에이전트 목록
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.List(context.Background())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// UpdateStatus POST /api/agents/{id}/status — 관리자용 에이전트 상태 변경
func (h *AgentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 에이전트 ID", http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	// 허용 상태만 통과 (agents.status: pending/active/blocked)
	switch req.Status {
	case "pending", "active", "blocked":
	default:
		http.Error(w, "status는 pending/active/blocked 중 하나여야 합니다", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateStatus(r.Context(), id, req.Status); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete DELETE /api/agents/{id} — 관리자용 에이전트 삭제
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 에이전트 ID", http.StatusBadRequest)
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
