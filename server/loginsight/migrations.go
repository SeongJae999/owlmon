package loginsight

import (
	"context"
	"database/sql"
)

// EnsureTables — log_insights / log_template_cache 테이블이 없으면 생성.
// 워커 시작 시 1회 호출. 이미 존재하면 no-op.
//
// infra/postgres/init.sql 을 건드리지 않는 이유:
//   - 기존 운영 DB는 init.sql 재실행되지 않음 (entrypoint 1회용)
//   - IF NOT EXISTS 로 무중단 적용 가능
//   - 운영자가 별도 마이그레이션 작업 안 해도 됨
func EnsureTables(ctx context.Context, db *sql.DB) error {
	for _, stmt := range ddlStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

var ddlStatements = []string{
	`CREATE TABLE IF NOT EXISTS log_insights (
		id              BIGSERIAL PRIMARY KEY,
		template_hash   TEXT NOT NULL,
		sample_log_ids  BIGINT[] NOT NULL DEFAULT '{}',
		host_name       TEXT,
		severity        TEXT NOT NULL,
		category        TEXT NOT NULL,
		summary_ko      TEXT NOT NULL,
		root_cause_ko   TEXT,
		action_ko       TEXT,
		needs_human     BOOLEAN NOT NULL DEFAULT FALSE,
		model_name      TEXT NOT NULL,
		prompt_tokens   INT,
		output_tokens   INT,
		latency_ms      INT,
		created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_log_insights_created
		ON log_insights(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_log_insights_host
		ON log_insights(host_name, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_log_insights_severity
		ON log_insights(severity, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_log_insights_template
		ON log_insights(template_hash)`,
	`CREATE TABLE IF NOT EXISTS log_template_cache (
		template_hash    TEXT PRIMARY KEY,
		last_insight_id  BIGINT NOT NULL REFERENCES log_insights(id) ON DELETE CASCADE,
		hit_count        INT NOT NULL DEFAULT 1,
		last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_template_cache_seen
		ON log_template_cache(last_seen_at)`,
}
