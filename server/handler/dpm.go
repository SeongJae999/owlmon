package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/dpm"
)

// DPMHandler는 DPM 인스턴스/메트릭 API를 처리합니다.
type DPMHandler struct {
	store  *db.DPMStore
	poller *dpm.Poller
}

func NewDPMHandler(store *db.DPMStore, poller *dpm.Poller) *DPMHandler {
	return &DPMHandler{store: store, poller: poller}
}

// ListInstances GET /api/dpm/instances — 비밀번호는 제외하고 반환
func (h *DPMHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]dpm.Instance, len(instances))
	for i, inst := range instances {
		inst.PasswordEnc = ""
		out[i] = inst
	}
	if out == nil {
		out = []dpm.Instance{}
	}
	writeJSON(w, out)
}

// AddInstance POST /api/dpm/instances
func (h *DPMHandler) AddInstance(w http.ResponseWriter, r *http.Request) {
	var inst dpm.Instance
	if err := json.NewDecoder(r.Body).Decode(&inst); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if msg := validateInstance(&inst); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// 평문 비밀번호를 암호화
	encrypted, err := h.poller.Cipher().Encrypt(inst.Password)
	if err != nil {
		http.Error(w, "비밀번호 암호화 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}
	inst.PasswordEnc = encrypted
	inst.Password = ""

	created, err := h.store.Add(context.Background(), inst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 즉시 1회 폴링
	go h.poller.TriggerOne(created)

	created.PasswordEnc = ""
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

// DeleteInstance DELETE /api/dpm/instances/{id}
func (h *DPMHandler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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

// DPMStatusItem은 인스턴스 + 최근 메트릭을 합친 응답입니다.
type DPMStatusItem struct {
	Instance dpm.Instance         `json:"instance"`
	Metrics  *dpm.InstanceMetrics `json:"metrics,omitempty"`
}

// GetStatus GET /api/dpm/status — 모든 인스턴스의 최근 메트릭
func (h *DPMHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	instances, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	latest := h.poller.GetLatest()

	items := make([]DPMStatusItem, 0, len(instances))
	for _, inst := range instances {
		inst.PasswordEnc = ""
		item := DPMStatusItem{Instance: inst}
		if r, ok := latest[inst.ID]; ok {
			m := r.Metrics
			item.Metrics = &m
		}
		items = append(items, item)
	}
	writeJSON(w, items)
}

// GetQueries GET /api/dpm/instances/{id}/queries — 최근 슬로우 쿼리 TOP N
func (h *DPMHandler) GetQueries(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	queries, err := h.store.LatestQueryStats(context.Background(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if queries == nil {
		queries = []dpm.QueryStat{}
	}
	writeJSON(w, queries)
}

// GetMetricsHistory GET /api/dpm/instances/{id}/metrics?hours=1
func (h *DPMHandler) GetMetricsHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	hours := queryInt(r.URL.Query(), "hours", 1)
	if hours <= 0 || hours > 168 {
		hours = 1
	}
	history, err := h.store.MetricsHistory(context.Background(), id, hours)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if history == nil {
		history = []dpm.InstanceMetrics{}
	}
	writeJSON(w, history)
}

// TriggerCheck POST /api/dpm/instances/{id}/check — 즉시 폴링
func (h *DPMHandler) TriggerCheck(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	instances, err := h.store.List(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, inst := range instances {
		if inst.ID == id {
			result := h.poller.TriggerOne(inst)
			writeJSON(w, result.Metrics)
			return
		}
	}
	http.Error(w, "인스턴스를 찾을 수 없음", http.StatusNotFound)
}

func validateInstance(i *dpm.Instance) string {
	if strings.TrimSpace(i.Name) == "" {
		return "이름은 필수입니다"
	}
	if strings.TrimSpace(i.Host) == "" {
		return "호스트는 필수입니다"
	}
	if strings.TrimSpace(i.Username) == "" {
		return "사용자명은 필수입니다"
	}
	if i.Password == "" {
		return "비밀번호는 필수입니다"
	}
	if i.DBType == "" {
		i.DBType = "postgres"
	}
	if i.DBType != "postgres" {
		return "현재는 PostgreSQL만 지원합니다"
	}
	if strings.TrimSpace(i.Database) == "" {
		return "데이터베이스 이름은 필수입니다"
	}
	if i.Port == 0 {
		i.Port = 5432
	}
	if i.PollIntervalSec == 0 {
		i.PollIntervalSec = 60
	}
	if i.PollIntervalSec < 30 {
		return "최소 폴링 주기는 30초입니다"
	}
	return ""
}
