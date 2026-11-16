package db

import (
	"context"
	"os"
	"testing"
	"time"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: synthetic_* 테이블을 지움)
func TestSyntheticStatsAll(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS synthetic_results, synthetic_monitors;
		CREATE TABLE synthetic_monitors (id BIGSERIAL PRIMARY KEY);
		CREATE TABLE synthetic_results (id BIGSERIAL PRIMARY KEY, monitor_id BIGINT REFERENCES synthetic_monitors(id) ON DELETE CASCADE,
			success BOOLEAN, status_code INT DEFAULT 0, response_time_ms INT, error TEXT DEFAULT '', checked_at TIMESTAMPTZ);
		INSERT INTO synthetic_monitors VALUES (1), (2), (3)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, r := range []struct {
		id  int64
		ok  bool
		ms  int
		ago time.Duration
	}{
		{1, true, 100, time.Hour}, {1, true, 300, time.Hour}, {1, false, 9000, time.Hour},
		{1, true, 5000, 30 * time.Hour}, // 24시간 밖
		{2, false, 0, time.Minute},
		{3, true, 50, 48 * time.Hour},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO synthetic_results (monitor_id, success, response_time_ms, checked_at) VALUES ($1,$2,$3,$4)`,
			r.id, r.ok, r.ms, now.Add(-r.ago)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := NewSyntheticStore(pool).StatsAll(ctx, 24)
	if err != nil {
		t.Fatal(err)
	}
	if s := got[1]; s.TotalChecks != 3 || s.FailedChecks != 1 || s.AvgLatencyMs != 200 || s.P95LatencyMs != 300 || int(s.UptimePct) != 66 {
		t.Errorf("monitor 1: %+v", s)
	}
	if s := got[2]; s.MonitorID != 2 || s.TotalChecks != 1 || s.UptimePct != 0 || s.AvgLatencyMs != 0 {
		t.Errorf("monitor 2: %+v", s)
	}
	if _, ok := got[3]; ok {
		t.Errorf("24시간 안 결과가 없으면 빠짐: %+v", got[3])
	}
}
