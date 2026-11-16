package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/prom"
	"github.com/seongJae/owlmon/server/snmp"
)

// SNMPHandler는 SNMP 장비 관리 API를 처리합니다.
type SNMPHandler struct {
	store         *db.SNMPDeviceStore
	poller        *snmp.Poller
	prometheusURL string
	promCache     cached[snmpProm]
}

func NewSNMPHandler(store *db.SNMPDeviceStore, poller *snmp.Poller, prometheusURL string) *SNMPHandler {
	return &SNMPHandler{store: store, poller: poller, prometheusURL: prometheusURL}
}

// ListDevices GET /api/snmp/devices
func (h *SNMPHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.store.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
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
	req.Name, req.IP = strings.TrimSpace(req.Name), strings.TrimSpace(req.IP)
	if req.Name == "" || req.IP == "" {
		http.Error(w, "장비 이름과 IP를 입력하세요", http.StatusBadRequest)
		return
	}
	// 포트를 붙인 값(1.2.3.4:161)이나 MAC 주소가 저장되면 폴링이 매번 실패해 계속 '다운'으로 보임
	if net.ParseIP(req.IP) == nil {
		http.Error(w, "IP 주소 형식이 아닙니다 (예: 192.168.0.1)", http.StatusBadRequest)
		return
	}
	if req.Community == "" {
		req.Community = "public"
	}
	if req.Port == 0 {
		req.Port = 161
	}
	// IP 중복은 DB 제약에 걸려 "서버 내부 오류"로만 보였고, 이름 중복은 구성도·알림이 이름으로 장비를 찾아 두 장비가 섞인다
	existing, err := h.store.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	for _, d := range existing {
		if d.IP == req.IP || d.Name == req.Name {
			http.Error(w, fmt.Sprintf("이미 등록된 장비입니다 (%s, %s)", d.Name, d.IP), http.StatusConflict)
			return
		}
	}

	dev, err := h.store.Add(r.Context(), snmp.Device{
		Name:      req.Name,
		IP:        req.IP,
		Community: req.Community,
		Port:      req.Port,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
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
	if req.Port == 0 {
		req.Port = 161
	}
	dev, err := h.store.Update(r.Context(), snmp.Device{ID: id, Name: req.Name, IP: req.IP, Community: req.Community, Port: req.Port})
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "장비를 찾을 수 없습니다", http.StatusNotFound)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
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
	if err := h.store.Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// 장비 목록·서버 직접 폴링 결과는 캐시하지 않음 — 등록·삭제와 폴링이 바로 보이게
type snmpProm struct {
	snaps  map[string]*prom.SNMPSnapshot
	latest map[string]prom.SNMPSample
	upErr  error
}

func (h *SNMPHandler) fetchProm() (snmpProm, error) {
	snaps, err := prom.SNMPStatusAll(h.prometheusURL)
	if err != nil {
		log.Printf("[snmp] prometheus 스냅샷 조회 실패: %v", err)
	}
	// 응답 여부는 알림과 같은 조회(수집기별 최신 샘플) — 스냅샷 값은 끊긴 수집기의 남은 시계열을 걸러 내지 않아 복구 뒤 몇 분간 틀림
	latest, upErr := prom.SNMPUpAll(h.prometheusURL)
	return snmpProm{snaps: snaps, latest: latest, upErr: upErr}, nil
}

// GetStatus GET /api/snmp/status — Prometheus 기반 통합 스냅샷
//
// 데이터 소스 — server 직접 폴링이든 agent SNMP 프록시든 모두 Prometheus(snmp_*)에 적재.
// 이 핸들러는 거기서 통합해서 반환 → 망분리 환경에서도 같은 UI로 동작.
//
// 기존 응답 구조와 호환 — 클라이언트 SNMPDashboard.tsx의 DeviceStatus 형식.
func (h *SNMPHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	devices, _ := h.store.List(r.Context())

	// 응답 — 클라이언트가 사용하는 DeviceStatus 형식과 호환
	type ifStat struct {
		Index    int     `json:"Index"`
		Name     string  `json:"Name"`
		OperUp   bool    `json:"OperUp"`
		InBytes  uint64  `json:"InBytes"`
		OutBytes uint64  `json:"OutBytes"`
		InBps    float64 `json:"InBps"`
		OutBps   float64 `json:"OutBps"`
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
		DownSince   *time.Time  `json:"DownSince,omitempty"` // 화면 "N분째"와 확인 유효성 판단에 씀 — 빠져 있어 늘 비어 보였음
		NoData      bool        `json:"NoData,omitempty"`    // 수집 경로가 끊겨 상태를 모름 — 알림도 보류 중이라 '응답 없음'으로 보이면 안 됨
		// 마지막으로 상태를 확인한 뒤 지난 초 (에이전트 새 샘플·서버 폴링 중 최근 것). CollectedAt은 응답 시각이라 실제 수집 시점을 알 수 없음.
		// 시각이 아니라 경과 초로 줌 — 서버와 브라우저 시계가 달라도 화면이 그대로 쓸 수 있게
		DataAgeSec *float64 `json:"DataAgeSec,omitempty"`
	}

	out := make([]devStat, 0, len(devices))
	nowStr := time.Now().Format(time.RFC3339)
	pr, _ := h.promCache.get(statusCacheTTL, h.fetchProm)
	latest, upErr := pr.latest, pr.upErr
	for _, d := range devices {
		snap := pr.snaps[d.Name]
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
		// 알림과 같은 판정 — 에이전트 수집 값이나 서버 직접 폴링 중 하나라도 응답하면 정상
		var at time.Time
		if upErr == nil {
			ds.Up = latest[d.Name].Up
			at = latest[d.Name].At
		}
		// 실패한 서버 폴링은 상태를 확인한 게 아님 — 그 시각으로 덮으면 에이전트 수집이 밀린 게 가려짐
		if m := h.poller.Status(d.ID); m != nil && m.Up && m.CollectedAt.After(at) {
			at = m.CollectedAt
		}
		if !at.IsZero() {
			age := max(0, time.Since(at).Seconds())
			ds.DataAgeSec = &age
		}
		ds.Up = h.poller.Up(d, map[string]bool{d.Name: ds.Up}, time.Now())
		if !ds.Up {
			if memStatus := h.poller.Status(d.ID); memStatus != nil && memStatus.LastError != "" {
				ds.LastError = memStatus.LastError
			}
			ds.DownSince = h.poller.DownSince(d.ID)
			ds.NoData = h.poller.NoData(d.ID)
		}
		out = append(out, ds)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
