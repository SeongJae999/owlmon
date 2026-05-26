package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/prom"
	"github.com/seongJae/owlmon/server/snmp"
)

// SNMPHandler는 SNMP 장비 관리 API를 처리합니다.
type SNMPHandler struct {
	store         *db.SNMPDeviceStore
	poller        *snmp.Poller
	prometheusURL string
}

func NewSNMPHandler(store *db.SNMPDeviceStore, poller *snmp.Poller, prometheusURL string) *SNMPHandler {
	return &SNMPHandler{store: store, poller: poller, prometheusURL: prometheusURL}
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

// UpdateDevice PUT /api/snmp/devices/{id}
func (h *SNMPHandler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
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
	dev := snmp.Device{ID: id, Name: req.Name, IP: req.IP, Community: req.Community, Port: req.Port}
	if err := h.store.Update(context.Background(), dev); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 즉시 재폴링 — 변경된 community/IP 즉시 반영
	go h.poller.Poll(dev)
	w.Header().Set("Content-Type", "application/json")
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

// GetStatus GET /api/snmp/status — Prometheus 기반 통합 스냅샷
//
// 데이터 소스 — server 직접 폴링이든 agent SNMP 프록시든 모두 Prometheus(snmp_*)에 적재.
// 이 핸들러는 거기서 통합해서 반환 → 망분리 환경에서도 같은 UI로 동작.
//
// 기존 응답 구조와 호환 — 클라이언트 SNMPDashboard.tsx의 DeviceStatus 형식.
func (h *SNMPHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	devices, _ := h.store.List(context.Background())

	// 응답 — 클라이언트가 사용하는 DeviceStatus 형식과 호환
	type ifStat struct {
		Index   int     `json:"Index"`
		Name    string  `json:"Name"`
		OperUp  bool    `json:"OperUp"`
		InBytes uint64  `json:"InBytes"`
		OutBytes uint64 `json:"OutBytes"`
		InBps   float64 `json:"InBps"`
		OutBps  float64 `json:"OutBps"`
	}
	type devStat struct {
		Device      snmp.Device `json:"Device"`
		Up          bool        `json:"Up"`
		UptimeSec   float64     `json:"UptimeSec"`
		Interfaces  []ifStat    `json:"Interfaces,omitempty"`
		CollectedAt string      `json:"CollectedAt"`
		LastError   string      `json:"LastError,omitempty"`
		ResponseMs  int64       `json:"ResponseMs,omitempty"`
		SysDescr    string      `json:"SysDescr,omitempty"`
	}

	out := make([]devStat, 0, len(devices))
	nowStr := time.Now().Format(time.RFC3339)
	for _, d := range devices {
		snap, _ := prom.SNMPStatusForDevice(h.prometheusURL, d.Name)
		ds := devStat{
			Device:      d,
			CollectedAt: nowStr,
		}
		if snap != nil {
			ds.Up = snap.Up
			ds.UptimeSec = snap.UptimeSec
			ds.ResponseMs = snap.ResponseMs
			ds.SysDescr = snap.SysDescr
			for _, ifc := range snap.Interfaces {
				ds.Interfaces = append(ds.Interfaces, ifStat{
					Index: ifc.Index, Name: ifc.Name, OperUp: ifc.OperUp,
					InBps: ifc.InBps, OutBps: ifc.OutBps,
				})
			}
		}
		// Prometheus에 데이터 없으면 — server 직접 폴링 결과(메모리)도 함께 확인
		if !ds.Up {
			if memStatus := h.poller.Status(d.ID); memStatus != nil && memStatus.LastError != "" {
				ds.LastError = memStatus.LastError
			}
		}
		out = append(out, ds)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
