package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Agent struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Status    string    `json:"status"` // pending, active, blocked
	CreatedAt time.Time `json:"created_at"`
	LastSeen  *time.Time `json:"last_seen"`
}

type AgentStore struct {
	pool *pgxpool.Pool
}

func NewAgentStore(pool *pgxpool.Pool) *AgentStore {
	return &AgentStore{pool: pool}
}

func (s *AgentStore) List(ctx context.Context) ([]Agent, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, key, status, created_at, last_seen FROM agents ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Key, &a.Status, &a.CreatedAt, &a.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, nil
}

func (s *AgentStore) GetByKey(ctx context.Context, key string) (*Agent, error) {
	var a Agent
	err := s.pool.QueryRow(ctx, "SELECT id, name, key, status, created_at, last_seen FROM agents WHERE key = $1", key).
		Scan(&a.ID, &a.Name, &a.Key, &a.Status, &a.CreatedAt, &a.LastSeen)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *AgentStore) Upsert(ctx context.Context, name, key string) error {
	_, err := s.pool.Exec(ctx, 
		`INSERT INTO agents (name, key, status) VALUES ($1, $2, 'pending')
		 ON CONFLICT (name) DO UPDATE SET key = EXCLUDED.key, last_seen = NOW()`,
		name, key)
	return err
}

func (s *AgentStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, "UPDATE agents SET status = $1 WHERE id = $2", status, id)
	return err
}

func (s *AgentStore) UpdateLastSeen(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, "UPDATE agents SET last_seen = NOW() WHERE key = $1", key)
	return err
}

func (s *AgentStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM agents WHERE id = $1", id)
	return err
}
