package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/seongJae/owlmon/server/auth"
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
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS sessions, users`)
	// last_active_at이 없던 이전 버전 테이블에서도 올라와야 함
	if _, err := pool.Exec(ctx, `CREATE TABLE sessions (id TEXT PRIMARY KEY, username TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL)`); err != nil {
		t.Fatal(err)
	}
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
	future := time.Now().Add(time.Hour)
	if err := s.CreateSession(ctx, "before-disable", "admin", future); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, users[0].ID, "admin", true); err != nil {
		t.Fatalf("관리자 2명일 때 중지 가능해야 함: %v", err)
	}
	if _, err := s.Current(ctx, "admin", "before-disable"); !errors.Is(err, auth.ErrSessionEnded) {
		t.Fatalf("중지된 계정은 거부돼야 함: %v", err)
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

	// 위 동시 중지에서 viewer1이 중지됐을 수 있으므로 활성 상태로 되돌림
	if err := s.Update(ctx, v.ID, "viewer", false); err != nil {
		t.Fatal(err)
	}
	active := func(sid string) bool {
		_, err := s.Current(ctx, "viewer1", sid)
		return err == nil
	}
	for _, sid := range []string{"a", "b", "c"} {
		if err := s.CreateSession(ctx, sid, "viewer1", future); err != nil {
			t.Fatal(err)
		}
	}
	if active("없는세션") {
		t.Fatal("없는 세션은 거부돼야 함")
	}
	if _, err := s.Current(ctx, "admin", "a"); err == nil {
		t.Fatal("다른 계정의 세션으로 통과하면 안 됨")
	}
	// 무조작 제한은 관리자만: 31분 지나면 관리자는 끊기고 조회 전용은 유지. 조작 신호가 오면 다시 유효
	if err := s.CreateSession(ctx, "adm", "admin", future); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Current(ctx, "admin", "adm"); err != nil {
		t.Fatalf("방금 로그인한 관리자는 유효: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET last_active_at = NOW() - interval '31 minutes'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Current(ctx, "admin", "adm"); !errors.Is(err, auth.ErrIdle) {
		t.Fatalf("관리자 무조작 끊김 기대: %v", err)
	}
	if !active("a") {
		t.Fatal("조회 전용은 무조작이어도 유지돼야 함")
	}
	if err := s.TouchSession(ctx, "adm"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Current(ctx, "admin", "adm"); err != nil {
		t.Fatalf("조작 후 유효 기대: %v", err)
	}
	// 연장하면 만료 시각이 지나도 유효
	if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at = NOW() - interval '1 minute' WHERE id = 'a'`); err != nil {
		t.Fatal(err)
	}
	if active("a") {
		t.Fatal("만료된 세션은 거부돼야 함")
	}
	if err := s.ExtendSession(ctx, "a", future); err != nil {
		t.Fatal(err)
	}
	if !active("a") {
		t.Fatal("연장 후 유효 기대")
	}
	// DB 장애는 세션 종료가 아니라 일시 오류로 구분돼야 함
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Current(cancelled, "admin", "adm"); err == nil || errors.Is(err, auth.ErrSessionEnded) {
		t.Fatalf("일시 오류 기대: %v", err)
	}
	// 로그아웃: 그 세션만 끊김
	if err := s.DeleteSession(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if active("a") || !active("b") || !active("c") {
		t.Fatal("로그아웃은 해당 세션만 끊어야 함")
	}
	// 본인 비밀번호 변경: 현재 세션만 유지
	if err := s.SetPassword(ctx, "viewer1", "hash3", "b"); err != nil {
		t.Fatal(err)
	}
	if !active("b") || active("c") {
		t.Fatal("비밀번호 변경 시 현재 세션만 남아야 함")
	}
	// 중지 후 재활성화해도 예전 세션은 되살아나지 않음
	if err := s.Update(ctx, v.ID, "viewer", true); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, v.ID, "viewer", false); err != nil {
		t.Fatal(err)
	}
	if active("b") {
		t.Fatal("재활성화 후 예전 세션은 무효여야 함")
	}
	// 만료 세션은 거부되고 다음 로그인 때 정리됨
	if err := s.CreateSession(ctx, "old", "viewer1", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if active("old") {
		t.Fatal("만료 세션은 거부돼야 함")
	}
	if err := s.CreateSession(ctx, "new", "viewer1", future); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE id = 'old'`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("만료 세션 정리 기대: %d %v", left, err)
	}
	// 비밀번호 초기화(keep 없음): 전부 끊김
	if err := s.SetPassword(ctx, "viewer1", "hash4", ""); err != nil {
		t.Fatal(err)
	}
	if active("new") {
		t.Fatal("비밀번호 초기화 시 모든 세션이 끊겨야 함")
	}
	if err := s.SetPassword(ctx, "nobody", "x", ""); err == nil {
		t.Fatal("없는 계정 비밀번호 변경은 실패해야 함")
	}
}
