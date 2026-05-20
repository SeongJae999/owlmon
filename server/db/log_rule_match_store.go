package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LogRuleMatch는 한 로그가 한 룰에 매칭된 기록.
type LogRuleMatch struct {
	ID        int64     `json:"id"`
	LogID     int64     `json:"log_id"`
	RuleID    int64     `json:"rule_id"`
	MatchedAt time.Time `json:"matched_at"`
	Host      string    `json:"host"`
	Severity  string    `json:"severity"`
}

type LogRuleMatchStore struct {
	pool *pgxpool.Pool
}

func NewLogRuleMatchStore(pool *pgxpool.Pool) *LogRuleMatchStore {
	return &LogRuleMatchStore{pool: pool}
}

// BatchInsert는 여러 매칭을 한 번에 INSERT한다 (ingest 핫패스용).
func (s *LogRuleMatchStore) BatchInsert(ctx context.Context, matches []LogRuleMatch) error {
	if len(matches) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("INSERT INTO log_rule_matches (log_id, rule_id, host, severity) VALUES ")
	args := make([]any, 0, len(matches)*4)
	for i, m := range matches {
		if i > 0 {
			sb.WriteString(",")
		}
		base := i * 4
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4)
		args = append(args, m.LogID, m.RuleID, m.Host, m.Severity)
	}
	_, err := s.pool.Exec(ctx, sb.String(), args...)
	return err
}

// CountByRule은 룰별 최근 시간창 안의 매칭 횟수 (threshold 평가용).
func (s *LogRuleMatchStore) CountByRule(ctx context.Context, ruleID int64, windowSec int) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM log_rule_matches
		WHERE rule_id = $1 AND matched_at > NOW() - ($2 || ' seconds')::interval`,
		ruleID, windowSec).Scan(&n)
	return n, err
}
