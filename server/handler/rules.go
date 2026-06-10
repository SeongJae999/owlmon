package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/rules"
)

// LogRule은 API I/O용 룰 표현 (DB 컬럼과 1:1 매핑).
type LogRule struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Pattern         string  `json:"pattern"`
	Severity        string  `json:"severity"`
	ThresholdCount  *int    `json:"threshold_count"`
	ThresholdWindow *int    `json:"threshold_window"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Enabled         bool    `json:"enabled"`
	Description     *string `json:"description"`
	Category        *string `json:"category"`
}

type RulesHandler struct {
	pool   *pgxpool.Pool
	engine *rules.Engine // 룰 변경 후 Reload 호출
}

func NewRulesHandler(pool *pgxpool.Pool, engine *rules.Engine) *RulesHandler {
	return &RulesHandler{pool: pool, engine: engine}
}

// List GET /api/log-rules — 전체 룰 (활성/비활성 모두)
func (h *RulesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, pattern, severity, threshold_count, threshold_window,
		       cooldown_seconds, enabled, description, category
		FROM log_rules ORDER BY category NULLS LAST, name`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	defer rows.Close()

	out := []LogRule{}
	for rows.Next() {
		var rl LogRule
		if err := rows.Scan(&rl.ID, &rl.Name, &rl.Pattern, &rl.Severity,
			&rl.ThresholdCount, &rl.ThresholdWindow, &rl.CooldownSeconds,
			&rl.Enabled, &rl.Description, &rl.Category); err != nil {
			respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
			return
		}
		out = append(out, rl)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Create POST /api/log-rules
func (h *RulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req LogRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	if err := validateRule(&req); err != nil {
		respondError(w, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	if req.CooldownSeconds <= 0 {
		req.CooldownSeconds = 300
	}

	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO log_rules (name, pattern, severity, threshold_count, threshold_window,
		                       cooldown_seconds, enabled, description, category)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		req.Name, req.Pattern, req.Severity, req.ThresholdCount, req.ThresholdWindow,
		req.CooldownSeconds, req.Enabled, req.Description, req.Category,
	).Scan(&req.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}

	h.reloadEngine()

	audit.Log(r.Context(), "rule.create", "rule", fmt.Sprintf("%d", req.ID),
		map[string]any{"name": req.Name, "severity": req.Severity, "pattern": req.Pattern})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(req)
}

// Update PUT /api/log-rules/{id}
func (h *RulesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	var req LogRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	if err := validateRule(&req); err != nil {
		respondError(w, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	if req.CooldownSeconds <= 0 {
		req.CooldownSeconds = 300
	}

	_, err = h.pool.Exec(r.Context(), `
		UPDATE log_rules SET
		  name=$2, pattern=$3, severity=$4,
		  threshold_count=$5, threshold_window=$6,
		  cooldown_seconds=$7, enabled=$8,
		  description=$9, category=$10,
		  updated_at=NOW()
		WHERE id=$1`,
		id, req.Name, req.Pattern, req.Severity, req.ThresholdCount, req.ThresholdWindow,
		req.CooldownSeconds, req.Enabled, req.Description, req.Category,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}

	h.reloadEngine()
	audit.Log(r.Context(), "rule.update", "rule", fmt.Sprintf("%d", id),
		map[string]any{"name": req.Name, "enabled": req.Enabled, "severity": req.Severity})
	w.WriteHeader(http.StatusNoContent)
}

// Toggle POST /api/log-rules/{id}/toggle — enabled 반전
func (h *RulesHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	_, err = h.pool.Exec(r.Context(),
		`UPDATE log_rules SET enabled = NOT enabled, updated_at = NOW() WHERE id=$1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	h.reloadEngine()
	audit.Log(r.Context(), "rule.toggle", "rule", fmt.Sprintf("%d", id), nil)
	w.WriteHeader(http.StatusNoContent)
}

// Delete DELETE /api/log-rules/{id}
func (h *RulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	_, err = h.pool.Exec(r.Context(), `DELETE FROM log_rules WHERE id=$1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	h.reloadEngine()
	audit.Log(r.Context(), "rule.delete", "rule", fmt.Sprintf("%d", id), nil)
	w.WriteHeader(http.StatusNoContent)
}

// MatchStats GET /api/log-rules/stats — 룰별 최근 24h 매칭 횟수
func (h *RulesHandler) MatchStats(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT rule_id, COUNT(*) FROM log_rule_matches
		WHERE matched_at > NOW() - INTERVAL '24 hours'
		GROUP BY rule_id`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	defer rows.Close()
	stats := map[int64]int{}
	for rows.Next() {
		var rid int64
		var n int
		if err := rows.Scan(&rid, &n); err == nil {
			stats[rid] = n
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

// MatchStatsDetailed GET /api/log-rules/stats/detailed
// 룰별 1h/24h/7d/30d 매칭 + 알림 발사 카운트 + 마지막 매칭 시각.
// 룰 매칭 통계 페이지의 데이터 소스.
type RuleStatDetail struct {
	RuleID        int64     `json:"rule_id"`
	Matches1h     int       `json:"matches_1h"`
	Matches24h    int       `json:"matches_24h"`
	Matches7d     int       `json:"matches_7d"`
	Matches30d   int       `json:"matches_30d"`
	AlertsFired  int       `json:"alerts_fired"`         // alert_history(category='log_rule')에 룰 이름으로 발사된 카운트 (전 기간)
	LastMatchAt  *string   `json:"last_match_at,omitempty"`
}

func (h *RulesHandler) MatchStatsDetailed(w http.ResponseWriter, r *http.Request) {
	// 1) 룰별 매칭 카운트 (1h/24h/7d/30d + last)
	rows, err := h.pool.Query(r.Context(), `
		SELECT rule_id,
		  COUNT(*) FILTER (WHERE matched_at > NOW() - INTERVAL '1 hour')   AS m1h,
		  COUNT(*) FILTER (WHERE matched_at > NOW() - INTERVAL '24 hours') AS m24h,
		  COUNT(*) FILTER (WHERE matched_at > NOW() - INTERVAL '7 days')   AS m7d,
		  COUNT(*) FILTER (WHERE matched_at > NOW() - INTERVAL '30 days')  AS m30d,
		  MAX(matched_at) AS last_at
		FROM log_rule_matches
		GROUP BY rule_id`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	defer rows.Close()

	out := map[int64]*RuleStatDetail{}
	for rows.Next() {
		var rid int64
		var m1h, m24h, m7d, m30d int
		var lastAt *time.Time
		if err := rows.Scan(&rid, &m1h, &m24h, &m7d, &m30d, &lastAt); err != nil {
			continue
		}
		d := &RuleStatDetail{RuleID: rid, Matches1h: m1h, Matches24h: m24h, Matches7d: m7d, Matches30d: m30d}
		if lastAt != nil {
			s := lastAt.Format(time.RFC3339)
			d.LastMatchAt = &s
		}
		out[rid] = d
	}

	// 2) 룰별 알림 발사 카운트 — alert_history의 subject가 룰 이름을 포함
	//    log_rules.name과 alert_history.subject(category='log_rule')를 매칭
	alertRows, err := h.pool.Query(r.Context(), `
		SELECT r.id, COUNT(h.id) AS fired
		FROM log_rules r
		LEFT JOIN alert_history h
		  ON h.category = 'log_rule' AND h.subject LIKE '%' || r.name || '%'
		GROUP BY r.id`)
	if err == nil {
		defer alertRows.Close()
		for alertRows.Next() {
			var rid int64
			var fired int
			if err := alertRows.Scan(&rid, &fired); err == nil {
				if d, ok := out[rid]; ok {
					d.AlertsFired = fired
				} else {
					out[rid] = &RuleStatDetail{RuleID: rid, AlertsFired: fired}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (h *RulesHandler) reloadEngine() {
	if h.engine != nil {
		go func() {
			_ = h.engine.Reload(context.Background())
		}()
	}
}

// validateRule은 입력 유효성을 검증한다.
func validateRule(r *LogRule) error {
	if r.Name == "" {
		return errors.New("name 필수")
	}
	if r.Pattern == "" {
		return errors.New("pattern 필수")
	}
	if _, err := regexp.Compile(r.Pattern); err != nil {
		return errors.New("유효하지 않은 정규식: " + err.Error())
	}
	switch r.Severity {
	case "info", "warning", "critical":
	default:
		return errors.New("severity는 info/warning/critical 중 하나")
	}
	return nil
}
