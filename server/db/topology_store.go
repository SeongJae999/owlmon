package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TopologyDevice struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Zone   string `json:"zone"`
	Parent string `json:"parent,omitempty"`
	Pair   string `json:"pair,omitempty"`
}

type TopologyLayout struct {
	Zones   []string         `json:"zones"`
	Devices []TopologyDevice `json:"devices"`
}

type TopologyStore struct {
	pool *pgxpool.Pool
}

func NewTopologyStore(pool *pgxpool.Pool) *TopologyStore {
	return &TopologyStore{pool: pool}
}

// init.sql은 새 DB에서만 돌기 때문에 기존 운영 DB에도 생기도록 기동 시 생성
func (s *TopologyStore) EnsureTable(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS topology_layout (
			id         INT PRIMARY KEY CHECK (id = 1),
			layout     JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	return err
}

func (s *TopologyStore) Get(ctx context.Context) (TopologyLayout, error) {
	l := TopologyLayout{Zones: []string{}, Devices: []TopologyDevice{}}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT layout FROM topology_layout WHERE id = 1`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(raw, &l)
}

func (s *TopologyStore) Save(ctx context.Context, l TopologyLayout) error {
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO topology_layout (id, layout, updated_at) VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET layout = EXCLUDED.layout, updated_at = NOW()`, raw)
	return err
}
