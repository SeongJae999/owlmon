package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// UpdateStatus POST /api/agents/{id}/status — 관리자용 에이전트 상태 변경
func (h *AgentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id") // TODO: stub — strconv.ParseInt 필요
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Delete DELETE /api/agents/{id} — 관리자용 에이전트 삭제
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
