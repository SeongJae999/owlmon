package db

import (
	"context"
	"os"
	"reflect"
	"testing"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: topology_layout 테이블을 지움)
func TestTopologyStore_SaveAndGet(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS topology_layout`); err != nil {
		t.Fatal(err)
	}
	s := NewTopologyStore(pool)
	if err := s.EnsureTable(ctx); err != nil {
		t.Fatal(err)
	}

	empty, err := s.Get(ctx)
	if err != nil || len(empty.Zones) != 0 || empty.Devices == nil || len(empty.Devices) != 0 {
		t.Fatalf("저장 전에는 빈 레이아웃 기대: %+v %v", empty, err)
	}

	want := TopologyLayout{
		Zones:   []string{"내부망"},
		Devices: []TopologyDevice{{Name: "코어", Kind: "network", Zone: "내부망"}, {Name: "web-01", Kind: "server", Zone: "내부망", Parent: "코어"}},
	}
	for range 2 {
		if err := s.Save(ctx, want); err != nil {
			t.Fatalf("저장은 덮어쓰기여야 함: %v", err)
		}
	}
	got, err := s.Get(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("저장한 값 그대로 기대: %+v %v", got, err)
	}
}
