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
