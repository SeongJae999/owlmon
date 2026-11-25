package db

import (
	"context"
	"errors"
	"os"
	"testing"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: work_orders 표를 지움)
func TestWorkOrderStore(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS work_orders`); err != nil {
		t.Fatal(err)
	}
	s := NewWorkOrderStore(pool)
	for range 2 {
		if err := s.EnsureTable(ctx); err != nil {
			t.Fatal(err)
		}
	}
	str := func(v string) *string { return &v }

	late, err := s.Create(ctx, WorkOrder{HostName: "web-01", Title: "디스크 교체", DueDate: str("2026-12-01"), Assignee: "김기사", CreatedBy: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if late.Status != "todo" || late.DueDate == nil || *late.DueDate != "2026-12-01" || late.CreatedBy != "admin" || late.DoneAt != nil {
		t.Fatalf("새 작업: %+v", late)
	}
	noDue, err := s.Create(ctx, WorkOrder{HostName: "sw-01", Title: "케이블 정리", AlertRef: str("alert-7")})
	if err != nil {
		t.Fatal(err)
	}
	if noDue.DueDate != nil || noDue.AlertRef == nil || *noDue.AlertRef != "alert-7" {
		t.Fatalf("기한 없는 작업: %+v", noDue)
	}
	soon, err := s.Create(ctx, WorkOrder{HostName: "web-01", Title: "펌웨어 업데이트", DueDate: str("2026-11-20")})
	if err != nil {
		t.Fatal(err)
	}

	// 기한 급한 순, 기한 없는 작업은 맨 뒤
	all, err := s.List(ctx, WorkOrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != soon.ID || all[1].ID != late.ID || all[2].ID != noDue.ID {
		t.Fatalf("순서: %+v", all)
	}
	if byHost, _ := s.List(ctx, WorkOrderFilter{Host: "web-01"}); len(byHost) != 2 {
		t.Fatalf("장비 필터: %+v", byHost)
	}

	// 내용 수정 — 기한을 지우면 기한 없음
	late.Title, late.DueDate = "디스크 2개 교체", nil
	up, err := s.Update(ctx, late)
	if err != nil || up.Title != "디스크 2개 교체" || up.DueDate != nil || up.Status != "todo" {
		t.Fatalf("수정: %+v %v", up, err)
	}

	// 끝남이면 완료 시각이 찍히고, 다시 열면 지워짐
	done, err := s.SetStatus(ctx, soon.ID, "done", "교체 완료")
	if err != nil || done.Status != "done" || done.DoneNote != "교체 완료" || done.DoneAt == nil {
		t.Fatalf("완료: %+v %v", done, err)
	}
	if open, _ := s.List(ctx, WorkOrderFilter{Status: "todo"}); len(open) != 2 {
		t.Fatalf("상태 필터: %+v", open)
	}
	again, err := s.SetStatus(ctx, soon.ID, "doing", "")
	if err != nil || again.DoneAt != nil {
		t.Fatalf("다시 열기: %+v %v", again, err)
	}

	if _, err := s.SetStatus(ctx, 99999, "done", ""); !errors.Is(err, ErrWorkOrderNotFound) {
		t.Fatalf("없는 작업 상태 변경: %v", err)
	}
	if _, err := s.Update(ctx, WorkOrder{ID: 99999, HostName: "x", Title: "x"}); !errors.Is(err, ErrWorkOrderNotFound) {
		t.Fatalf("없는 작업 수정: %v", err)
	}
	if err := s.Delete(ctx, noDue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, noDue.ID); !errors.Is(err, ErrWorkOrderNotFound) {
		t.Fatalf("없는 작업 삭제: %v", err)
	}
}
