package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 그룹에 넣을 수 있는 장비 종류 — ref는 서버만 host_name(DB에 서버 표가 없음), 나머지는 각 표의 id. SSL은 domain(UNIQUE)
var GroupKinds = map[string]bool{"server": true, "snmp": true, "synthetic": true, "ssl": true, "dpm": true}

type Group struct {
	ID       int64    `json:"id"`
	ParentID *int64   `json:"parent_id"`
	Name     string   `json:"name"`
	Sort     int      `json:"sort"`
	IPRanges []string `json:"ip_ranges"` // 이 대역의 장비는 직접 지정하지 않아도 이 그룹으로 보임 (화면이 매칭)
}

type GroupMember struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	GroupID int64  `json:"group_id"`
}

type GroupTree struct {
	Groups  []Group       `json:"groups"`
	Members []GroupMember `json:"members"`
}

const MaxIPRanges = 100

// GroupError는 사용자에게 그대로 보여 줄 수 있는 거부 사유
type GroupError struct{ Msg string }

func (e *GroupError) Error() string { return e.Msg }

type GroupStore struct {
	pool *pgxpool.Pool
}

func NewGroupStore(pool *pgxpool.Pool) *GroupStore {
	return &GroupStore{pool: pool}
}

// init.sql은 새 DB에서만 돌기 때문에 기존 운영 DB에도 생기도록 기동 시 생성.
// 장비는 그룹 하나에만 속함 — 상태를 위로 합산할 때 두 번 세지 않게
func (s *GroupStore) EnsureTable(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS device_groups (
			id        BIGSERIAL PRIMARY KEY,
			parent_id BIGINT REFERENCES device_groups(id),
			name      TEXT NOT NULL CHECK (name <> ''),
			sort      INT  NOT NULL DEFAULT 0
		);
		CREATE UNIQUE INDEX IF NOT EXISTS device_groups_sibling_name ON device_groups (COALESCE(parent_id, 0), name);
		CREATE TABLE IF NOT EXISTS group_members (
			kind     TEXT   NOT NULL,
			ref      TEXT   NOT NULL,
			group_id BIGINT NOT NULL REFERENCES device_groups(id) ON DELETE CASCADE,
			PRIMARY KEY (kind, ref)
		);
		CREATE INDEX IF NOT EXISTS group_members_group ON group_members (group_id);
		ALTER TABLE device_groups ADD COLUMN IF NOT EXISTS ip_ranges TEXT[] NOT NULL DEFAULT '{}'`)
	return err
}

func (s *GroupStore) Tree(ctx context.Context) (GroupTree, error) {
	t := GroupTree{Groups: []Group{}, Members: []GroupMember{}}
	rows, err := s.pool.Query(ctx, `SELECT id, parent_id, name, sort, ip_ranges FROM device_groups ORDER BY sort, name`)
	if err != nil {
		return t, err
	}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.ParentID, &g.Name, &g.Sort, &g.IPRanges); err != nil {
			rows.Close()
			return t, err
		}
		t.Groups = append(t.Groups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return t, err
	}
	rows, err = s.pool.Query(ctx, `SELECT kind, ref, group_id FROM group_members`)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.Kind, &m.Ref, &m.GroupID); err != nil {
			return t, err
		}
		t.Members = append(t.Members, m)
	}
	return t, rows.Err()
}

// nil 슬라이스는 NULL로 들어가 NOT NULL 제약에 걸림
func ranges(g Group) []string {
	if g.IPRanges == nil {
		return []string{}
	}
	return g.IPRanges
}

func (s *GroupStore) Create(ctx context.Context, g Group) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO device_groups (parent_id, name, sort, ip_ranges) VALUES ($1, $2, $3, $4) RETURNING id`,
		g.ParentID, g.Name, g.Sort, ranges(g)).Scan(&id)
	return id, groupErr(err)
}

// Update는 이름·순서·상위 그룹을 바꿈. 자기 자신이나 하위 그룹 아래로 옮기면 거부 — 트리가 고리가 되어 화면이 끝없이 돎
func (s *GroupStore) Update(ctx context.Context, g Group) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// 동시에 두 그룹을 서로의 아래로 옮기는 경우를 막으려고 표 전체를 잠금 (그룹 수정은 드묾)
	if _, err := tx.Exec(ctx, `LOCK TABLE device_groups IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	if g.ParentID != nil {
		var cyc bool
		err := tx.QueryRow(ctx, `
			WITH RECURSIVE up(id, parent_id) AS (
				SELECT id, parent_id FROM device_groups WHERE id = $1
				UNION ALL
				SELECT g.id, g.parent_id FROM device_groups g JOIN up ON g.id = up.parent_id
			)
			SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, *g.ParentID, g.ID).Scan(&cyc)
		if err != nil {
			return err
		}
		if cyc {
			return &GroupError{"그룹을 자기 자신이나 하위 그룹 아래로 옮길 수 없습니다"}
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE device_groups SET parent_id = $2, name = $3, sort = $4, ip_ranges = $5 WHERE id = $1`, g.ID, g.ParentID, g.Name, g.Sort, ranges(g))
	if err != nil {
		return groupErr(err)
	}
	if tag.RowsAffected() == 0 {
		return &GroupError{"그룹이 없습니다"}
	}
	return tx.Commit(ctx)
}

// Delete는 그룹을 지우고 하위 그룹·장비·IP 대역을 한 단계 위로 올림 — 그룹 하나를 지웠다고 아래 장비들이 트리에서 사라지지 않게.
// 맨 위 그룹이면 장비는 '미지정'이 됨
func (s *GroupStore) Delete(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Update와 같은 표 잠금 — 삭제와 이동이 엇갈려 서로를 기다리는 교착을 막음
	if _, err := tx.Exec(ctx, `LOCK TABLE device_groups IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	var parent *int64
	if err := tx.QueryRow(ctx, `SELECT parent_id FROM device_groups WHERE id = $1`, id).Scan(&parent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &GroupError{"그룹이 없습니다"}
		}
		return err
	}
	// 올라간 하위 그룹 이름이 위쪽 형제와 겹치면 고유 제약에 걸림 — 그대로 오류로 알림
	if _, err := tx.Exec(ctx, `UPDATE device_groups SET parent_id = $2 WHERE parent_id = $1`, id, parent); err != nil {
		return groupErr(err)
	}
	if parent != nil {
		if _, err := tx.Exec(ctx, `UPDATE group_members SET group_id = $2 WHERE group_id = $1`, id, *parent); err != nil {
			return err
		}
		// IP 대역으로 들어온 장비도 직접 지정한 장비처럼 위 그룹에 남게 대역을 올림
		var n int
		if err := tx.QueryRow(ctx, `UPDATE device_groups SET ip_ranges = ARRAY(
				SELECT DISTINCT unnest(ip_ranges || (SELECT ip_ranges FROM device_groups WHERE id = $1)))
			WHERE id = $2 RETURNING cardinality(ip_ranges)`, id, *parent).Scan(&n); err != nil {
			return err
		}
		// 넘긴 채 저장하면 위 그룹을 고칠 때마다 개수 제한에 걸려 아무것도 바꿀 수 없음
		if n > MaxIPRanges {
			return &GroupError{fmt.Sprintf("위 그룹의 IP 대역이 %d개를 넘게 되어 지울 수 없습니다. 대역을 먼저 줄이세요", MaxIPRanges)}
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM device_groups WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Assign은 장비들을 그룹에 넣음. groupID가 nil이면 소속을 뺌('미지정')
func (s *GroupStore) Assign(ctx context.Context, groupID *int64, items []GroupMember) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, m := range items {
		if groupID == nil {
			_, err = tx.Exec(ctx, `DELETE FROM group_members WHERE kind = $1 AND ref = $2`, m.Kind, m.Ref)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO group_members (kind, ref, group_id) VALUES ($1, $2, $3)
				ON CONFLICT (kind, ref) DO UPDATE SET group_id = EXCLUDED.group_id`, m.Kind, m.Ref, *groupID)
		}
		if err != nil {
			return groupErr(err)
		}
	}
	return tx.Commit(ctx)
}

func groupErr(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return &GroupError{"같은 위치에 같은 이름의 그룹이 있습니다"}
		case "23503":
			return &GroupError{"상위 그룹이 없습니다"}
		}
	}
	return err
}
