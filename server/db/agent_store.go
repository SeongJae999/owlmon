package db

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Agent — 키 원문·해시는 담지 않는다 (목록 API로 다시 나가지 않게).
type Agent struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"` // active, blocked (pending은 예전 등록 방식의 흔적)
	CreatedAt time.Time  `json:"created_at"`
	LastSeen  *time.Time `json:"last_seen"`
}

// ErrTokenInvalid — 없거나, 이미 쓰였거나, 만료된 등록 토큰.
var ErrTokenInvalid = errors.New("등록 토큰이 없거나 이미 쓰였거나 만료됨")

type AgentStore struct {
	pool *pgxpool.Pool
}

func NewAgentStore(pool *pgxpool.Pool) *AgentStore {
	return &AgentStore{pool: pool}
}

// HashSecret — 에이전트 키·등록 토큰은 DB에 sha256 해시로만 둔다 (DB가 새어도 키로 쓸 수 없게).
func HashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// EnsureTables — 예전 DB(평문 key 열)를 key_hash로 옮기고 등록 토큰 표를 만든다. 여러 번 불러도 안전.
func (s *AgentStore) EnsureTables(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS agents (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			key_hash TEXT UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen TIMESTAMPTZ
		);
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS key_hash TEXT UNIQUE;
		DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'agents' AND column_name = 'key') THEN
				UPDATE agents SET key_hash = encode(sha256(convert_to(key, 'UTF8')), 'hex') WHERE key_hash IS NULL;
				ALTER TABLE agents DROP COLUMN key;
			END IF;
		END $$;
		CREATE TABLE IF NOT EXISTS agent_enroll_tokens (
			token_hash TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ
		);
		DELETE FROM agent_enroll_tokens WHERE expires_at < NOW() - INTERVAL '30 days';`)
	return err
}

func (s *AgentStore) List(ctx context.Context) ([]Agent, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, status, created_at, last_seen FROM agents ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Agent{}
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Status, &a.CreatedAt, &a.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetByKey — 해시로 찾은 뒤 해시를 상수시간으로 한 번 더 비교한다.
func (s *AgentStore) GetByKey(ctx context.Context, key string) (*Agent, error) {
	want := HashSecret(key)
	var a Agent
	var got string
	err := s.pool.QueryRow(ctx, "SELECT id, name, key_hash, status, created_at, last_seen FROM agents WHERE key_hash = $1", want).
		Scan(&a.ID, &a.Name, &got, &a.Status, &a.CreatedAt, &a.LastSeen)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return nil, pgx.ErrNoRows
	}
	return &a, nil
}

// CreateEnrollToken — 관리자가 서버 이름으로 일회용 등록 토큰을 만든다.
func (s *AgentStore) CreateEnrollToken(ctx context.Context, name, token, createdBy string, ttl time.Duration) (time.Time, error) {
	expires := time.Now().Add(ttl)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_enroll_tokens (token_hash, name, created_by, expires_at) VALUES ($1, $2, $3, $4)`,
		HashSecret(token), name, createdBy, expires)
	return expires, err
}

// TokenUsable — 쓰이지 않았고 만료 전인 토큰인지 (설치 중 바이너리 다운로드용, 소진하지 않음).
func (s *AgentStore) TokenUsable(ctx context.Context, token string) bool {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM agent_enroll_tokens WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW())`,
		HashSecret(token)).Scan(&ok)
	return err == nil && ok
}

// Enroll — 토큰을 소진하고 그 이름의 에이전트에 새 키(해시)를 묶어 활성으로 만든다.
// 같은 이름이 있으면(재설치·폐기 후 재등록) 키를 바꿔 예전 키는 바로 무효가 된다.
// prev는 등록 전 상태("" = 새 장비, active = 키 교체, blocked = 폐기된 장비를 되살림) — 감사 기록용.
func (s *AgentStore) Enroll(ctx context.Context, token, newKey string) (name, prev string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`UPDATE agent_enroll_tokens SET used_at = NOW()
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		 RETURNING name`, HashSecret(token)).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrTokenInvalid
	}
	if err != nil {
		return "", "", err
	}
	if err := tx.QueryRow(ctx, "SELECT status FROM agents WHERE name = $1 FOR UPDATE", name).Scan(&prev); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO agents (name, key_hash, status) VALUES ($1, $2, 'active')
		 ON CONFLICT (name) DO UPDATE SET key_hash = EXCLUDED.key_hash, status = 'active', last_seen = NULL`,
		name, HashSecret(newKey)); err != nil {
		return "", "", err
	}
	return name, prev, tx.Commit(ctx)
}

// Name — 감사 기록용.
func (s *AgentStore) Name(ctx context.Context, id int64) string {
	var name string
	_ = s.pool.QueryRow(ctx, "SELECT name FROM agents WHERE id = $1", id).Scan(&name)
	return name
}

// UpdateStatus — 폐기(blocked)면 그 이름으로 발급해 둔 미사용 등록 토큰도 소진 처리한다
// (유출된 설치 명령으로 폐기한 장비를 다시 살리지 못하게). 한 문장이라 원자적.
func (s *AgentStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `
		WITH a AS (UPDATE agents SET status = $1 WHERE id = $2 RETURNING name)
		UPDATE agent_enroll_tokens SET used_at = NOW()
		WHERE $1 = 'blocked' AND used_at IS NULL AND name IN (SELECT name FROM a)`, status, id)
	return err
}

func (s *AgentStore) UpdateLastSeen(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, "UPDATE agents SET last_seen = NOW() WHERE id = $1", id)
	return err
}

// Delete — 폐기와 같은 이유로 그 이름의 미사용 등록 토큰도 소진 처리한다.
func (s *AgentStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		WITH a AS (DELETE FROM agents WHERE id = $1 RETURNING name)
		UPDATE agent_enroll_tokens SET used_at = NOW()
		WHERE used_at IS NULL AND name IN (SELECT name FROM a)`, id)
	return err
}
