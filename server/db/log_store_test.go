package db

import (
	"context"
	"os"
	"strconv"
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
}
