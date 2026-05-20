package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []LogRule{}
	for rows.Next() {
		var rl LogRule
		if err := rows.Scan(&rl.ID, &rl.Name, &rl.Pattern, &rl.Severity,
			&rl.ThresholdCount, &rl.ThresholdWindow, &rl.CooldownSeconds,
			&rl.Enabled, &rl.Description, &rl.Category); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateRule(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.reloadEngine()

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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateRule(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.reloadEngine()
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.reloadEngine()
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.reloadEngine()
	w.WriteHeader(http.StatusNoContent)
}

// MatchStats GET /api/log-rules/stats — 룰별 최근 24h 매칭 횟수
func (h *RulesHandler) MatchStats(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT rule_id, COUNT(*) FROM log_rule_matches
		WHERE matched_at > NOW() - INTERVAL '24 hours'
		GROUP BY rule_id`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
