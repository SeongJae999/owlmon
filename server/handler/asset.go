package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
)

type AssetHandler struct {
	store *db.AssetStore
}

func NewAssetHandler(store *db.AssetStore) *AssetHandler {
	return &AssetHandler{store: store}
}

// List는 모든 자산 정보를 반환합니다.
// GET /api/assets
func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	assets, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if assets == nil {
		assets = []db.Asset{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assets)
}

// Upsert는 자산을 생성하거나 업데이트합니다.
// PUT /api/assets
func (h *AssetHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var a db.Asset
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	if a.HostName == "" {
		http.Error(w, "host_name은 필수입니다", http.StatusBadRequest)
		return
	}
	result, err := h.store.Upsert(context.Background(), a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Delete는 자산을 삭제합니다.
// DELETE /api/assets/{id}
func (h *AssetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
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
