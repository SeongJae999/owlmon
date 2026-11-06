package db

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/snmp"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: snmp_devices 테이블을 지움)
func TestSNMPDeviceUpdate_KeepsCommunityWhenEmpty(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS snmp_devices;
		CREATE TABLE snmp_devices (id BIGSERIAL PRIMARY KEY, name TEXT, ip TEXT, community TEXT, port INT, created_at TIMESTAMPTZ DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	s := NewSNMPDeviceStore(pool)
	d, err := s.Add(ctx, snmp.Device{Name: "sw", IP: "10.0.0.1", Community: "s3cret", Port: 161})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Update(ctx, snmp.Device{ID: d.ID, Name: "sw2", IP: "10.0.0.2", Port: 162})
	if err != nil || got.Community != "s3cret" {
		t.Fatalf("빈 값이면 기존 community 유지 기대: %q %v", got.Community, err)
	}
	if got, err = s.Update(ctx, snmp.Device{ID: d.ID, Name: "sw2", IP: "10.0.0.2", Community: "new", Port: 162}); err != nil || got.Community != "new" {
		t.Fatalf("입력하면 변경 기대: %q %v", got.Community, err)
	}
	if _, err := s.Update(ctx, snmp.Device{ID: d.ID + 99}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("없는 장비는 ErrNoRows 기대: %v", err)
	}
}
