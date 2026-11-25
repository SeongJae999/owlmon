package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/db"
)

type WorkOrderHandler struct {
	store *db.WorkOrderStore
}

func NewWorkOrderHandler(store *db.WorkOrderStore) *WorkOrderHandler {
	return &WorkOrderHandler{store: store}
}

// GET /api/work-orders?status=todo|doing|done&host=장비이름
func (h *WorkOrderHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := db.WorkOrderFilter{Status: q.Get("status"), Host: strings.TrimSpace(q.Get("host"))}
	if f.Status != "" && !db.WorkOrderStatuses[f.Status] {
		http.Error(w, "알 수 없는 상태입니다", http.StatusBadRequest)
		return
	}
	list, err := h.store.List(r.Context(), f)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	writeJSON(w, list)
}

// 관리자가 보내는 작업 내용. 상태·작성자·시각은 서버가 정함
type workOrderInput struct {
	HostName    string  `json:"host_name"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	DueDate     *string `json:"due_date"`
	Assignee    string  `json:"assignee"`
	AlertRef    *string `json:"alert_ref"`
}

func readWorkOrder(w http.ResponseWriter, r *http.Request) (db.WorkOrder, bool) {
	var in workOrderInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return db.WorkOrder{}, false
	}
	wo, msg := normalizeWorkOrder(in)
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return wo, false
	}
	return wo, true
}

// normalizeWorkOrder는 앞뒤 공백을 지우고 길이·날짜를 확인. 거부 사유는 화면에 그대로 보여 줄 문구
func normalizeWorkOrder(in workOrderInput) (db.WorkOrder, string) {
	wo := db.WorkOrder{
		HostName:    strings.TrimSpace(in.HostName),
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		Assignee:    strings.TrimSpace(in.Assignee),
	}
	switch {
	case wo.HostName == "" || utf8.RuneCountInString(wo.HostName) > 200:
		return wo, "장비를 고르세요"
	case wo.Title == "" || utf8.RuneCountInString(wo.Title) > 100:
		return wo, "할 일은 1~100자로 적으세요"
	case utf8.RuneCountInString(wo.Description) > 2000:
		return wo, "자세한 내용은 2000자까지 적을 수 있습니다"
	case utf8.RuneCountInString(wo.Assignee) > 60:
		return wo, "담당자는 60자까지 적을 수 있습니다"
	}
	if in.DueDate != nil {
		if d := strings.TrimSpace(*in.DueDate); d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				return wo, "기한 날짜 형식이 아닙니다 (예: 2026-12-31)"
			}
			wo.DueDate = &d
		}
	}
	if in.AlertRef != nil {
		if a := strings.TrimSpace(*in.AlertRef); a != "" {
			if utf8.RuneCountInString(a) > 200 {
				return wo, "알림 참조가 너무 깁니다"
			}
			wo.AlertRef = &a
		}
	}
	return wo, ""
}

// POST /api/work-orders (관리자)
func (h *WorkOrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	wo, ok := readWorkOrder(w, r)
	if !ok {
		return
	}
	wo.CreatedBy = auth.UsernameFromContext(r.Context())
	out, err := h.store.Create(r.Context(), wo)
	if !h.writeErr(w, err) {
		writeJSON(w, out)
	}
}

// PUT /api/work-orders/{id} (관리자) — 화면이 현재 값을 모두 보냄. 상태는 바꾸지 않음
func (h *WorkOrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := workOrderID(w, r)
	if !ok {
		return
	}
	wo, ok := readWorkOrder(w, r)
	if !ok {
		return
	}
	wo.ID = id
	out, err := h.store.Update(r.Context(), wo)
	if !h.writeErr(w, err) {
		writeJSON(w, out)
	}
}

// POST /api/work-orders/{id}/status — {status, done_note}. 현장 엔지니어(조회 계정)도 처리 결과를 남길 수 있음
func (h *WorkOrderHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := workOrderID(w, r)
	if !ok {
		return
	}
	var req struct {
		Status   string `json:"status"`
		DoneNote string `json:"done_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	if !db.WorkOrderStatuses[req.Status] {
		http.Error(w, "알 수 없는 상태입니다", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(req.DoneNote)
	if utf8.RuneCountInString(note) > 2000 {
		http.Error(w, "처리 메모는 2000자까지 적을 수 있습니다", http.StatusBadRequest)
		return
	}
	out, err := h.store.SetStatus(r.Context(), id, req.Status, note)
	if !h.writeErr(w, err) {
		writeJSON(w, out)
	}
}

// DELETE /api/work-orders/{id} (관리자)
func (h *WorkOrderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := workOrderID(w, r)
	if !ok {
		return
	}
	if !h.writeErr(w, h.store.Delete(r.Context(), id)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func workOrderID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (h *WorkOrderHandler) writeErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrWorkOrderNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
	}
	return true
}
