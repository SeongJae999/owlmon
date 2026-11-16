package db

import (
	"context"
	"errors"
	"os"
	"testing"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: 그룹 표를 지움)
func TestGroupStore(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS group_members; DROP TABLE IF EXISTS device_groups`); err != nil {
		t.Fatal(err)
	}
	s := NewGroupStore(pool)
	for range 2 {
		if err := s.EnsureTable(ctx); err != nil {
			t.Fatal(err)
		}
	}
	mk := func(parent *int64, name string) int64 {
		t.Helper()
		id, err := s.Create(ctx, Group{ParentID: parent, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	hq := mk(nil, "본사")
	floor := mk(&hq, "3층")
	room := mk(&floor, "전산실")
	var gErr *GroupError
	if _, err := s.Create(ctx, Group{ParentID: &hq, Name: "3층"}); !errors.As(err, &gErr) {
		t.Fatalf("같은 위치 같은 이름: %v", err)
	}
	if _, err := s.Create(ctx, Group{Name: "3층"}); err != nil {
		t.Fatalf("다른 위치면 같은 이름 허용: %v", err)
	}

	// 자기 하위로 옮기면 고리 — 거부
	if err := s.Update(ctx, Group{ID: hq, ParentID: &room, Name: "본사"}); !errors.As(err, &gErr) {
		t.Fatalf("고리: %v", err)
	}
	if err := s.Update(ctx, Group{ID: hq, ParentID: &hq, Name: "본사"}); !errors.As(err, &gErr) {
		t.Fatalf("자기 자신: %v", err)
	}

	if err := s.Assign(ctx, &room, []GroupMember{{Kind: "server", Ref: "web1"}, {Kind: "snmp", Ref: "7"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Assign(ctx, &floor, []GroupMember{{Kind: "server", Ref: "web1"}}); err != nil { // 한 그룹에만 속함 — 옮겨짐
		t.Fatal(err)
	}

	// 중간 그룹을 지우면 하위 그룹·장비가 한 단계 위로
	if err := s.Delete(ctx, floor); err != nil {
		t.Fatal(err)
	}
	tr, err := s.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parent := map[int64]*int64{}
	for _, g := range tr.Groups {
		parent[g.ID] = g.ParentID
	}
	if p := parent[room]; p == nil || *p != hq {
		t.Fatalf("전산실이 본사 아래로 올라가야 함: %v", p)
	}
	where := map[string]int64{}
	for _, m := range tr.Members {
		where[m.Kind+":"+m.Ref] = m.GroupID
	}
	if where["server:web1"] != hq || where["snmp:7"] != room || len(where) != 2 {
		t.Fatalf("소속: %v", where)
	}

	// 맨 위 그룹을 지우면 장비는 미지정
	if err := s.Delete(ctx, hq); err != nil {
		t.Fatal(err)
	}
	if err := s.Assign(ctx, nil, []GroupMember{{Kind: "snmp", Ref: "7"}}); err != nil {
		t.Fatal(err)
	}
	tr, _ = s.Tree(ctx)
	if len(tr.Members) != 0 {
		t.Fatalf("미지정이어야 함: %v", tr.Members)
	}
	if err := s.Delete(ctx, hq); !errors.As(err, &gErr) {
		t.Fatalf("없는 그룹: %v", err)
	}
}
