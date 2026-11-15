package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Rack struct {
	Name   string `json:"name"`
	Room   string `json:"room"`   // 전산실·통신실 등 — 화면에서 같은 방끼리 묶음
	Height int    `json:"height"` // 칸 수 (U)
}

type RackStore struct {
	pool *pgxpool.Pool
}

func NewRackStore(pool *pgxpool.Pool) *RackStore {
	return &RackStore{pool: pool}
}

func (s *RackStore) List(ctx context.Context) ([]Rack, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, room, height FROM racks ORDER BY room, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	racks := []Rack{}
	for rows.Next() {
		var r Rack
		if err := rows.Scan(&r.Name, &r.Room, &r.Height); err != nil {
			return nil, err
		}
		racks = append(racks, r)
	}
	return racks, rows.Err()
}

// 높이를 줄여 이미 넣은 장비가 랙 밖으로 나가면 거부 — 장비가 화면에서 사라짐
func (s *RackStore) Save(ctx context.Context, r Rack) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`INSERT INTO racks (name, room, height) VALUES ($1, $2, $3)
		 ON CONFLICT (name) DO UPDATE SET room = EXCLUDED.room, height = EXCLUDED.height`,
		r.Name, r.Room, r.Height); err != nil {
		return err
	}
	var host string
	var unit, size int
	err = tx.QueryRow(ctx,
		`SELECT host_name, rack_unit, rack_size FROM assets WHERE rack=$1 AND rack_unit + rack_size - 1 > $2 ORDER BY rack_unit DESC LIMIT 1`,
		r.Name, r.Height).Scan(&host, &unit, &size)
	if err == nil {
		return &RackError{"높이를 줄이면 " + host + "(" + UnitRange(unit, size) + ")가 랙 밖으로 나갑니다"}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return tx.Commit(ctx)
}

// 장비가 들어 있는 랙은 지우지 않음 — 장비들이 위치를 잃음.
// 랙 행을 먼저 잠가, 같은 순간 이 랙에 장비를 넣는 저장(Upsert도 같은 행을 잠금)이 끝난 뒤에 장비 수를 센다
func (s *RackStore) Delete(ctx context.Context, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM racks WHERE name=$1 FOR UPDATE`, name); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE rack=$1`, name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return &RackError{"장비가 들어 있는 랙은 지울 수 없습니다 — 먼저 장비를 다른 랙으로 옮기거나 랙을 비우세요"}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM racks WHERE name=$1`, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
