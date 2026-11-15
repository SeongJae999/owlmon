package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/seongJae/owlmon/server/db"
)

type RackHandler struct {
	store *db.RackStore
}

func NewRackHandler(store *db.RackStore) *RackHandler {
	return &RackHandler{store: store}
}

// GET /api/racks
func (h *RackHandler) List(w http.ResponseWriter, r *http.Request) {
	racks, err := h.store.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	writeJSON(w, racks)
}

// PUT /api/racks — 이름 기준으로 추가·수정
func (h *RackHandler) Save(w http.ResponseWriter, r *http.Request) {
	var rk db.Rack
	if err := json.NewDecoder(r.Body).Decode(&rk); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	rk.Name, rk.Room = strings.TrimSpace(rk.Name), strings.TrimSpace(rk.Room)
	if rk.Name == "" {
		http.Error(w, "랙 이름을 입력하세요", http.StatusBadRequest)
		return
	}
	if rk.Height < 1 || rk.Height > 60 {
		http.Error(w, "랙 높이는 1~60U 사이로 입력하세요", http.StatusBadRequest)
		return
	}
	if !h.writeErr(w, h.store.Save(r.Context(), rk)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// DELETE /api/racks?name= — 한글 랙 이름이 경로에 들어가면 인코딩된 채로 넘어올 수 있어 쿼리로 받음
func (h *RackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.writeErr(w, h.store.Delete(r.Context(), r.URL.Query().Get("name"))) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *RackHandler) writeErr(w http.ResponseWriter, err error) bool {
	var rackErr *db.RackError
	switch {
	case err == nil:
		return false
	case errors.As(err, &rackErr):
		http.Error(w, rackErr.Msg, http.StatusConflict)
	default:
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
	}
	return true
}
