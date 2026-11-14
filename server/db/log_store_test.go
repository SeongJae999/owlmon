package db

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: logs 테이블을 비움)
func TestLogStore_IngestKeepsAllRows(t *testing.T) {
	dsn := os.Getenv("OWLMON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("OWLMON_TEST_PG_DSN 미설정")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `TRUNCATE logs CASCADE`); err != nil {
		t.Fatal(err)
	}
	s := NewLogStore(pool)

	recs := make([]LogRecord, 1500)
	for i := range recs {
		recs[i] = LogRecord{Timestamp: time.Now(), Host: "h", Source: "s", Line: strconv.Itoa(i)}
	}
	ids, err := s.Ingest(ctx, recs)
	if err != nil || len(ids) != 1500 {
		t.Fatalf("1500건 전부 저장돼야 함: ids=%d err=%v", len(ids), err)
	}

	// UTF-16LE 로그(MSSQL ERRORLOG 등)를 바이트 단위로 읽으면 0x00이 섞임 — 배치 전체가 거부되면 안 됨
	ids, err = s.Ingest(ctx, []LogRecord{
		{Timestamp: time.Now(), Host: "h", Source: "s", Line: "E\x00R\x00R\x00O\x00R\x00"},
		{Timestamp: time.Now(), Host: "h", Source: "s", Line: "ok"},
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("NUL 섞인 배치도 저장돼야 함: ids=%d err=%v", len(ids), err)
	}
	var line string
	if err := pool.QueryRow(ctx, `SELECT line FROM logs WHERE id=$1`, ids[0]).Scan(&line); err != nil || line != "ERROR" {
		t.Fatalf("NUL 제거된 라인 기대: %q %v", line, err)
	}
}

// 기존 운영 DB처럼 인덱스가 없는 상태에서 EnsureIndexes가 정리 DELETE와 CASCADE 둘 다 인덱스를 타게 만드는지 확인
func TestLogStore_CleanupWithIndexes(t *testing.T) {
	dsn := os.Getenv("OWLMON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("OWLMON_TEST_PG_DSN 미설정")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, q := range []string{
		`TRUNCATE logs CASCADE`,
		`DROP INDEX IF EXISTS idx_logs_created_at`,
		`DROP INDEX IF EXISTS idx_log_rule_matches_log`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	s := NewLogStore(pool)
	for i := 0; i < 2; i++ {
		if err := s.EnsureIndexes(ctx); err != nil {
			t.Fatalf("EnsureIndexes %d회차: %v", i+1, err)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) // t.Fatal 시 연결이 안 돌아오면 pool.Close()가 영원히 기다린다
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	for q, idx := range map[string]string{
		`EXPLAIN DELETE FROM logs WHERE created_at < now()`:     "idx_logs_created_at",
		`EXPLAIN DELETE FROM log_rule_matches WHERE log_id = 1`: "idx_log_rule_matches_log",
	} {
		rows, err := tx.Query(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(line + "\n")
		}
		rows.Close()
		if !strings.Contains(plan.String(), idx) {
			t.Fatalf("%s 를 타야 함:\n%s", idx, plan.String())
		}
	}
	tx.Rollback(ctx)

	var oldID, ruleID int64
	if err := pool.QueryRow(ctx, `INSERT INTO logs (timestamp, host, source, line, created_at)
		VALUES (now(), 'h', 's', 'old', now() - interval '31 days') RETURNING id`).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO logs (timestamp, host, source, line) VALUES (now(), 'h', 's', 'new')`); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO log_rules (name, pattern) VALUES ('cleanup-test', 'old')
		ON CONFLICT (name) DO UPDATE SET pattern = EXCLUDED.pattern RETURNING id`).Scan(&ruleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO log_rule_matches (log_id, rule_id, host, severity) VALUES ($1, $2, 'h', 'warning')`, oldID, ruleID); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.Cleanup(ctx, 30)
	if err != nil || deleted != 1 {
		t.Fatalf("30일 지난 1건만 지워야 함: deleted=%d err=%v", deleted, err)
	}
	var logs, matches int
	pool.QueryRow(ctx, `SELECT count(*) FROM logs`).Scan(&logs)
	pool.QueryRow(ctx, `SELECT count(*) FROM log_rule_matches WHERE log_id = $1`, oldID).Scan(&matches)
	if logs != 1 || matches != 0 {
		t.Fatalf("최근 로그 1건 남고 매칭 이력은 CASCADE로 지워져야 함: logs=%d matches=%d", logs, matches)
	}
}
