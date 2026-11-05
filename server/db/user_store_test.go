package db

import (
	"context"
	"errors"
	"os"
	"testing"
)

// 실제 PostgreSQL 필요 — OWLMON_TEST_PG_DSN이 없으면 건너뜀 (빈 테스트 DB를 가리킬 것: users 테이블을 지움)
func TestUserStore(t *testing.T) {
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
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS users`)
	s := NewUserStore(pool)

	// 첫 실행: 환경변수 관리자가 첫 관리자로 들어감, 두 번째 실행엔 중복 안 들어감
	for i := 0; i < 2; i++ {
		if err := s.EnsureTable(ctx, "admin", "hash1"); err != nil {
			t.Fatal(err)
		}
	}
	users, _ := s.List(ctx)
	if len(users) != 1 || users[0].Username != "admin" || users[0].Role != "admin" {
		t.Fatalf("시드 관리자 1명 기대: %+v", users)
	}
	if hash, role, disabled, err := s.ForLogin(ctx, "admin"); err != nil || hash != "hash1" || role != "admin" || disabled {
		t.Fatalf("ForLogin: %v %s %s %v", err, hash, role, disabled)
	}

	v, err := s.Create(ctx, "viewer1", "hash2", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "viewer1", "x", "viewer"); err == nil {
		t.Fatal("중복 아이디는 실패해야 함")
	}

	// 유일한 관리자는 중지·강등 불가
	if err := s.Update(ctx, users[0].ID, "admin", true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("마지막 관리자 중지 거부 기대, got %v", err)
	}
	if err := s.Update(ctx, users[0].ID, "viewer", false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("마지막 관리자 강등 거부 기대, got %v", err)
	}
	// 관리자가 둘이면 하나는 중지 가능
	if err := s.Update(ctx, v.ID, "admin", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, users[0].ID, "admin", true); err != nil {
		t.Fatalf("관리자 2명일 때 중지 가능해야 함: %v", err)
	}
	if role, active := s.Current(ctx, "admin"); active || role != "admin" {
		t.Fatalf("중지된 계정은 active=false: %s %v", role, active)
	}
	// 관리자 2명을 동시에 중지해도 최소 1명은 남아야 함 (FOR UPDATE 잠금)
	if err := s.Update(ctx, users[0].ID, "admin", false); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	for _, id := range []int64{users[0].ID, v.ID} {
		go func(id int64) { errs <- s.Update(ctx, id, "admin", true) }(id)
	}
	if e1, e2 := <-errs, <-errs; (e1 == nil) == (e2 == nil) {
		t.Fatalf("동시 중지 중 정확히 하나만 성공해야 함: %v / %v", e1, e2)
	}
	_ = s.Update(ctx, users[0].ID, "admin", false)

	if err := s.SetPassword(ctx, "viewer1", "hash3"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, "nobody", "x"); err == nil {
		t.Fatal("없는 계정 비밀번호 변경은 실패해야 함")
	}
}
