package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/seongJae/owlmon/server/report"
)

// ReportHandler는 월간 보고서 API를 처리합니다.
type ReportHandler struct {
	reporter *report.Reporter
}

func NewReportHandler(reporter *report.Reporter) *ReportHandler {
	return &ReportHandler{reporter: reporter}
}

// Preview는 보고서 데이터를 JSON으로 반환합니다 (이메일 미발송).
// GET /api/report/preview?year=2026&month=2
func (h *ReportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	year, month := parseYearMonth(r)

	rep, err := h.reporter.Generate(year, month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rep)
}

// Send는 보고서를 생성하여 이메일로 발송합니다.
// POST /api/report/send  body: {"year":2026,"month":2}  (생략 시 지난달)
func (h *ReportHandler) Send(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Year  int `json:"year"`
		Month int `json:"month"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// 기본값: 지난달
	if req.Year == 0 || req.Month == 0 {
		prev := time.Now().AddDate(0, -1, 0)
		req.Year = prev.Year()
		req.Month = int(prev.Month())
	}

	if err := h.reporter.SendReport(req.Year, time.Month(req.Month)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// parseYearMonth는 쿼리 파라미터에서 년/월을 파싱합니다. 없으면 지난달.
func parseYearMonth(r *http.Request) (int, time.Month) {
	q := r.URL.Query()
	now := time.Now()
	prev := now.AddDate(0, -1, 0)

	year := prev.Year()
	month := prev.Month()

	if y := q.Get("year"); y != "" {
		var yi int
		if _, err := fmt.Sscanf(y, "%d", &yi); err == nil {
			year = yi
		}
	}
	if m := q.Get("month"); m != "" {
		var mi int
		if _, err := fmt.Sscanf(m, "%d", &mi); err == nil && mi >= 1 && mi <= 12 {
			month = time.Month(mi)
		}
	}
	return year, month
}
