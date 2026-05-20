// Package rules는 로그 라인에 사용자 정의 정규식 룰을 평가한다.
//
// 동작 흐름:
//  1. 서버 시작 시 DB의 log_rules 전체를 메모리로 로드 (정규식 컴파일 캐시)
//  2. 로그 ingest 시 라인마다 모든 enabled 룰을 평가
//  3. 매칭되면 log_rule_matches에 기록
//  4. 빈도 임계치 충족 시 알림 트리거 (cooldown 고려)
//
// 룰셋 변경 시 Reload() 호출 — 폴링 또는 변경 알림으로 재로드.
package rules

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Rule은 컴파일된 룰의 인메모리 표현.
type Rule struct {
	ID              int64
	Name            string
	Severity        string
	ThresholdCount  int    // 0이면 1회 매칭으로도 알림
	ThresholdWindow int    // 초
	CooldownSeconds int
	Category        string
	Description     string
	compiled        *regexp.Regexp
}

// Match는 한 로그 라인의 매칭 결과.
type Match struct {
	RuleID          int64
	Name            string
	Severity        string
	ThresholdCount  int // 0이면 1회 매칭으로도 알림
	ThresholdWindow int // 초
	CooldownSeconds int
}

// Engine은 룰셋을 캐시하고 매칭을 수행한다.
type Engine struct {
	pool  *pgxpool.Pool
	mu    sync.RWMutex
	rules []Rule
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{pool: pool}
}

// Reload는 DB에서 enabled 룰을 다시 로드하고 정규식을 컴파일한다.
// 에러가 있는 룰(잘못된 정규식)은 로깅만 하고 스킵한다.
func (e *Engine) Reload(ctx context.Context) error {
	rows, err := e.pool.Query(ctx, `
		SELECT id, name, pattern, severity, threshold_count, threshold_window,
		       cooldown_seconds, category, description
		FROM log_rules WHERE enabled = true
		ORDER BY id`)
	if err != nil {
		return fmt.Errorf("룰 로드 실패: %w", err)
	}
	defer rows.Close()

	var compiled []Rule
	for rows.Next() {
		var r Rule
		var pattern string
		var thCount, thWindow *int
		var category, description *string
		if err := rows.Scan(&r.ID, &r.Name, &pattern, &r.Severity,
			&thCount, &thWindow, &r.CooldownSeconds, &category, &description); err != nil {
			continue
		}

		re, cerr := regexp.Compile(pattern)
		if cerr != nil {
			// 잘못된 정규식 — 운영자가 UI에서 수정해야. 일단 스킵.
			continue
		}
		r.compiled = re
		if thCount != nil {
			r.ThresholdCount = *thCount
		}
		if thWindow != nil {
			r.ThresholdWindow = *thWindow
		}
		if category != nil {
			r.Category = *category
		}
		if description != nil {
			r.Description = *description
		}
		compiled = append(compiled, r)
	}

	e.mu.Lock()
	e.rules = compiled
	e.mu.Unlock()
	return nil
}

// Evaluate는 한 로그 라인에 대해 매칭된 모든 룰을 반환한다.
// 짧고 핫패스 (ingest당 호출) — 정규식만 돈다, DB 접근 없음.
func (e *Engine) Evaluate(line string) []Match {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matches []Match
	for _, r := range e.rules {
		if r.compiled.MatchString(line) {
			matches = append(matches, Match{
				RuleID:          r.ID,
				Name:            r.Name,
				Severity:        r.Severity,
				ThresholdCount:  r.ThresholdCount,
				ThresholdWindow: r.ThresholdWindow,
				CooldownSeconds: r.CooldownSeconds,
			})
		}
	}
	return matches
}

// ShouldAlert는 룰 정책상 지금 알림을 보내야 하는지 판단한다.
//   1) cooldown — 마지막 알림 후 cooldownSec 안 지났으면 false
//   2) threshold — thCount/thWindow 둘 다 > 0이면 최근 thWindow초 안 매칭 횟수 >= thCount일 때만 true.
//      한 쪽이라도 0/null이면 threshold 평가 skip (1회 매칭으로도 알림)
//
// recentNewCount: 이 ingest 배치에서 방금 INSERT한 신규 매칭 수.
//   pgxpool의 별도 connection visibility 지연으로 DB COUNT가 신규 row를 못 볼
//   경우를 대비 — max(DB COUNT, newCount)로 보정.
func (e *Engine) ShouldAlert(ctx context.Context, ruleID int64, cooldownSec, thCount, thWindow, recentNewCount int) (bool, error) {
	// 1. cooldown
	var lastAt *time.Time
	_ = e.pool.QueryRow(ctx,
		`SELECT last_alerted_at FROM log_rule_alert_history WHERE rule_id = $1`,
		ruleID).Scan(&lastAt)
	if lastAt != nil && time.Since(*lastAt) < time.Duration(cooldownSec)*time.Second {
		return false, nil
	}

	// 2. threshold (옵션)
	if thCount > 0 && thWindow > 0 {
		var dbCount int
		err := e.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM log_rule_matches
			WHERE rule_id = $1 AND matched_at > NOW() - ($2 || ' seconds')::interval`,
			ruleID, thWindow).Scan(&dbCount)
		if err != nil {
			return false, err
		}
		// visibility race 보정 — 신규 매칭이 아직 DB에서 안 보일 수 있으니 max로
		count := dbCount
		if recentNewCount > count {
			count = recentNewCount
		}
		if count < thCount {
			return false, nil
		}
	}

	return true, nil
}

// RecordAlert는 알림 발송 시각을 갱신한다 (cooldown 평가용).
func (e *Engine) RecordAlert(ctx context.Context, ruleID int64) error {
	_, err := e.pool.Exec(ctx, `
		INSERT INTO log_rule_alert_history (rule_id, last_alerted_at)
		VALUES ($1, NOW())
		ON CONFLICT (rule_id) DO UPDATE SET last_alerted_at = NOW()`,
		ruleID)
	return err
}

// RuleCount는 현재 메모리에 로드된 enabled 룰 수.
func (e *Engine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}
