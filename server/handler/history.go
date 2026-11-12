package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/seongJae/owlmon/server/db"
)

type HistoryHandler struct {
	store *db.AlertHistoryStore
}

func NewHistoryHandler(store *db.AlertHistoryStore) *HistoryHandler {
	return &HistoryHandler{store: store}
}

// parseFilter — 쿼리 파라미터 → AlertHistoryFilter
func parseFilter(r *http.Request) db.AlertHistoryFilter {
	q := r.URL.Query()
	f := db.AlertHistoryFilter{
		Severity: q.Get("severity"),
		Host:     q.Get("host"),
		Limit:    500,
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = t
		}
	}
	return f
}

// List는 알림 히스토리를 검색합니다 (필터 + 시간 범위 지원).
func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	records, err := h.store.Search(r.Context(), parseFilter(r))
	if err != nil {
		http.Error(w, "히스토리 조회 실패", http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []db.AlertRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// Export GET /api/alert/history/export?format=csv|json — 감사 자료용 다운로드
func (h *HistoryHandler) Export(w http.ResponseWriter, r *http.Request) {
	filter := parseFilter(r)
	if filter.Limit < 1000 {
		filter.Limit = 10000
	}
	records, err := h.store.Search(r.Context(), filter)
	if err != nil {
		http.Error(w, "히스토리 조회 실패", http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")
	filename := fmt.Sprintf("owlmon-alerts-%s", time.Now().Format("20060102-150405"))

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.json\"", filename))
		json.NewEncoder(w).Encode(records)
		return
	}
	// CSV (엑셀 한글 호환 BOM)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.csv\"", filename))
	w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	defer cw.Flush()
	// 감사 자료로 엑셀에서 바로 읽도록 헤더·등급을 한국어로 (JSON 내보내기는 원래 값 유지)
	cw.Write([]string{"발송시각", "호스트", "분류", "등급", "제목", "내용"})
	for _, rec := range records {
		cw.Write([]string{
			rec.SentAt.Local().Format("2006-01-02 15:04:05"),
			rec.Host,
			rec.Category,
			severityKo(rec.Severity),
			rec.Subject,
			rec.Body,
		})
	}
}

// severityKo는 사용자에게 보이는 출력(메일·CSV·AI 입력)의 등급 표기 — 화면(web/src/api/rules.ts severityLabel)과 같은 기준
func severityKo(s string) string {
	switch s {
	case "critical":
		return "위험"
	case "warning":
		return "주의"
	case "info":
		return "정보"
	}
	return s
}
