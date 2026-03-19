package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/snmp"
)

// SNMPHandler는 SNMP 장비 관리 API를 처리합니다.
type SNMPHandler struct {
	store  *db.SNMPDeviceStore
	poller *snmp.Poller
}

func NewSNMPHandler(store *db.SNMPDeviceStore, poller *snmp.Poller) *SNMPHandler {
	return &SNMPHandler{store: store, poller: poller}
}

// ListDevices GET /api/snmp/devices
func (h *SNMPHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []snmp.Device{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// AddDevice POST /api/snmp/devices
func (h *SNMPHandler) AddDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		IP        string `json:"ip"`
		Community string `json:"community"`
		Port      int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if req.Community == "" {
		req.Community = "public"
	}
	if req.Port == 0 {
		req.Port = 161
	}

	dev, err := h.store.Add(context.Background(), snmp.Device{
		Name:      req.Name,
		IP:        req.IP,
		Community: req.Community,
		Port:      req.Port,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 즉시 첫 폴링
	go h.poller.Poll(dev)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(dev)
}

// DeleteDevice DELETE /api/snmp/devices/{id}
func (h *SNMPHandler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
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

// GetStatus GET /api/snmp/status — 모든 장비의 현재 상태 반환
func (h *SNMPHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	statuses := h.poller.Statuses()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}
