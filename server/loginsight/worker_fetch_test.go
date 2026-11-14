package loginsight

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: logs 테이블을 비움)
func TestFetchRecentLogs_LateArrivalNotSkipped(t *testing.T) {
	dsn := os.Getenv("OWLMON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("OWLMON_TEST_PG_DSN 미설정")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `TRUNCATE logs CASCADE`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	worker := New(Config{Window: time.Minute, SeverityFilter: []string{"error"}}, pool, nil, nil)
	ins := func(line string, ts, created time.Time) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO logs (timestamp, host, source, line, level, created_at)
			VALUES ($1,'h','s',$2,'ERROR',$3) RETURNING id`, ts, line, created).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	fetch := func() []string {
		lines, err := worker.fetchRecentLogs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, l := range lines {
			out = append(out, l.Message)
		}
		return out
	}

	ins("before-window", now.Add(-3*time.Minute), now.Add(-3*time.Minute))
	ins("in-window", now.Add(-50*time.Second), now.Add(-50*time.Second))
	if got := fetch(); !slices.Equal(got, []string{"in-window"}) {
		t.Fatalf("1차 tick: %q", got)
	}

	// 에이전트가 5분 전에 읽은 줄이 WAL 재전송으로 이제 도착
	ins("late", now.Add(-5*time.Minute), now.Add(-40*time.Second))
	// 방금 들어온 줄은 아직 커밋 순서가 확정되지 않았으니 다음 tick으로 미뤄도 됨 — 단, 빠지면 안 됨
	fresh := ins("fresh", now, now)
	if got := fetch(); !slices.Equal(got, []string{"late"}) {
		t.Fatalf("2차 tick: 늦게 도착한 줄이 잡혀야 함: %q", got)
	}
	pool.Exec(ctx, `UPDATE logs SET created_at = $1 WHERE id = $2`, now.Add(-40*time.Second), fresh)
	if got := fetch(); !slices.Equal(got, []string{"fresh"}) {
		t.Fatalf("3차 tick: %q", got)
	}
}

// 한 tick에 한도(5000줄)를 넘게 쌓여도 남은 줄은 다음 tick에 이어 읽어야 함 (WAL 재전송이 몰릴 때).
func TestFetchRecentLogs_OverLimitContinuesNextTick(t *testing.T) {
	dsn := os.Getenv("OWLMON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("OWLMON_TEST_PG_DSN 미설정")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `TRUNCATE logs CASCADE`); err != nil {
		t.Fatal(err)
	}
	worker := New(Config{Window: time.Minute, SeverityFilter: []string{"error"}}, pool, nil, nil)
	if _, err := pool.Exec(ctx, `INSERT INTO logs (timestamp, host, source, line, level, created_at)
		SELECT NOW(), 'h', 's', 'line-' || g, 'ERROR', NOW() - interval '40 seconds'
		FROM generate_series(1, $1) g`, fetchLimit+1); err != nil {
		t.Fatal(err)
	}
	first, err := worker.fetchRecentLogs(ctx)
	if err != nil || len(first) != fetchLimit {
		t.Fatalf("1차 tick: %d건 %v", len(first), err)
	}
	rest, err := worker.fetchRecentLogs(ctx)
	if err != nil || len(rest) != 1 || rest[0].Message != "line-5001" {
		t.Fatalf("2차 tick에 남은 1건 기대: %+v %v", rest, err)
	}
}

func TestStore_SaveKeepsInputText(t *testing.T) {
	dsn := os.Getenv("OWLMON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("OWLMON_TEST_PG_DSN 미설정")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := EnsureTables(ctx, pool); err != nil {
		t.Fatal(err)
	}
	ins := &Insight{TemplateHash: "input-text-test", SampleLogIDs: []int64{1}, Severity: SeverityLow, Category: CatApp,
		SummaryKo: "요약", ModelName: "m", CreatedAt: time.Now(), InputText: "Host: h\nSample log lines:\n[1] 원문"}
	if err := NewStore(pool).Save(ctx, ins); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM log_insights WHERE id = $1`, ins.ID)
	var got string
	if err := pool.QueryRow(ctx, `SELECT input_text FROM log_insights WHERE id = $1`, ins.ID).Scan(&got); err != nil || got != ins.InputText {
		t.Fatalf("input_text 저장 기대: %q err=%v", got, err)
	}
}
