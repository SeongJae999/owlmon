package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/seongJae/owlmon/server/audit"
)

// AuditHandler — 감사 로그 조회 API.
type AuditHandler struct {
	store *audit.Store
}

func NewAuditHandler(store *audit.Store) *AuditHandler {
	return &AuditHandler{store: store}
}

// List GET /api/audit?actor=&action=&target_type=&from=&to=&limit=
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, "감사 로그 저장소가 비활성화되어 있습니다 (DB 미연결)", http.StatusServiceUnavailable)
		return
	}
	f := parseAuditFilter(r)
	entries, err := h.store.Search(r.Context(), f)
	if err != nil {
		http.Error(w, "조회 실패", http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []audit.ListEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// Export GET /api/audit/export?format=csv|json — 감사 자료 다운로드
func (h *AuditHandler) Export(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, "감사 로그 저장소 비활성", http.StatusServiceUnavailable)
		return
	}
	f := parseAuditFilter(r)
	if f.Limit < 1000 {
		f.Limit = 10000
	}
	entries, err := h.store.Search(r.Context(), f)
	if err != nil {
		http.Error(w, "조회 실패", http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")
	filename := fmt.Sprintf("owlmon-audit-%s", time.Now().Format("20060102-150405"))

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.json\"", filename))
		json.NewEncoder(w).Encode(entries)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.csv\"", filename))
	w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	cw := csv.NewWriter(w)
	defer cw.Flush()
	cw.Write([]string{"ts", "actor", "ip", "action", "target_type", "target_id", "result", "details"})
	for _, e := range entries {
		cw.Write([]string{
			e.TS, e.Actor, e.IP, e.Action, e.TargetType, e.TargetID, e.Result, string(e.Details),
		})
	}
}

func parseAuditFilter(r *http.Request) audit.SearchFilter {
	q := r.URL.Query()
	f := audit.SearchFilter{
		Actor:      q.Get("actor"),
		Action:     q.Get("action"),
		TargetType: q.Get("target_type"),
		From:       q.Get("from"),
		To:         q.Get("to"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	return f
}
