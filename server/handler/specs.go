package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/db"
)

type SpecsHandler struct {
	store *db.SpecsStore
}

func NewSpecsHandler(store *db.SpecsStore) *SpecsHandler {
	return &SpecsHandler{store: store}
}

// 에이전트가 보내는 페이로드 (agent/specs/specs.go의 Specs와 매칭).
// 별도 타입으로 받아서 JSON 컬럼은 RawMessage로 그대로 통과시킨다.
type specsPayload struct {
	HostName         string          `json:"host_name"`
	CPU              cpuPayload      `json:"cpu"`
	MemoryTotalBytes int64           `json:"memory_total_bytes"`
	Disks            json.RawMessage `json:"disks"`
	Networks         json.RawMessage `json:"networks"`
	DiskTopology     json.RawMessage `json:"disk_topology"`
	OSPrettyName     string          `json:"os_pretty_name"`
	KernelVersion    string          `json:"kernel_version"`
	Virtualization   string          `json:"virtualization"`
	Arch             string          `json:"arch"`
}

type cpuPayload struct {
	Model   string `json:"model"`
	Cores   int    `json:"cores"`
	Sockets int    `json:"sockets"`
}

// Ingest POST /api/agent/specs — 에이전트가 시작 시 1회 호출
func (h *SpecsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var p specsPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "잘못된 요청 본문", http.StatusBadRequest)
		return
	}
	if p.HostName == "" {
		http.Error(w, "host_name 누락", http.StatusBadRequest)
		return
	}

	// disks/networks가 비어있으면 빈 배열로 (DB JSONB null 회피)
	if len(p.Disks) == 0 {
		p.Disks = json.RawMessage("[]")
	}
	if len(p.Networks) == 0 {
		p.Networks = json.RawMessage("[]")
	}
	if len(p.DiskTopology) == 0 {
		p.DiskTopology = json.RawMessage("[]")
	}

	spec := &db.AgentSpecs{
		HostName:         p.HostName,
		CPUModel:         p.CPU.Model,
		CPUCores:         p.CPU.Cores,
		CPUSockets:       p.CPU.Sockets,
		MemoryTotalBytes: p.MemoryTotalBytes,
		Disks:            p.Disks,
		Networks:         p.Networks,
		DiskTopology:     p.DiskTopology,
		OSPrettyName:     p.OSPrettyName,
		KernelVersion:    p.KernelVersion,
		Virtualization:   p.Virtualization,
		Arch:             p.Arch,
	}

	if err := h.store.Upsert(context.Background(), spec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List GET /api/agent/specs — 모든 호스트 스펙
func (h *SpecsHandler) List(w http.ResponseWriter, r *http.Request) {
	specs, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if specs == nil {
		specs = []db.AgentSpecs{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(specs)
}

// Get GET /api/agent/specs/{host} — 단일 호스트 스펙
func (h *SpecsHandler) Get(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if host == "" {
		http.Error(w, "host 파라미터 누락", http.StatusBadRequest)
		return
	}

	spec, err := h.store.Get(context.Background(), host)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "해당 호스트 스펙 없음", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}
