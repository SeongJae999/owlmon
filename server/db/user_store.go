package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLastAdmin = errors.New("마지막 관리자 계정은 중지하거나 권한을 낮출 수 없습니다")

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

// init.sql은 DB 최초 생성 때만 돌기 때문에 운영 DB에는 서버가 직접 생성.
// 비어 있으면 기존 환경변수 관리자를 첫 관리자로 넣어 기존 로그인이 그대로 동작하게 함
func (s *UserStore) EnsureTable(ctx context.Context, seedUser, seedHash string) error {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id            BIGSERIAL PRIMARY KEY,
			username      TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL CHECK (role IN ('admin', 'viewer')),
			disabled      BOOLEAN NOT NULL DEFAULT false,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			username   TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
			expires_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (username, password_hash, role)
		SELECT $1, $2, 'admin' WHERE NOT EXISTS (SELECT 1 FROM users)`,
		seedUser, seedHash)
	return err
}

func (s *UserStore) ForLogin(ctx context.Context, username string) (hash, role string, disabled bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT password_hash, role, disabled FROM users WHERE username = $1`, username,
	).Scan(&hash, &role, &disabled)
	return
}

// 토큰은 8시간 유효하므로 요청마다 확인해야 로그아웃·사용 중지·역할 변경이 즉시 반영됨
func (s *UserStore) Current(ctx context.Context, username, sessionID string) (role string, active bool) {
	var disabled bool
	if err := s.pool.QueryRow(ctx, `
		SELECT u.role, u.disabled FROM users u
		JOIN sessions s ON s.username = u.username
		WHERE u.username = $1 AND s.id = $2 AND s.expires_at > NOW()`, username, sessionID,
	).Scan(&role, &disabled); err != nil {
		return "", false
	}
	return role, !disabled
}

// 만료된 세션은 별도 작업 없이 로그인할 때 함께 지움
func (s *UserStore) CreateSession(ctx context.Context, id, username string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		WITH expired AS (DELETE FROM sessions WHERE expires_at <= NOW())
		INSERT INTO sessions (id, username, expires_at) VALUES ($1, $2, $3)`, id, username, expiresAt)
	return err
}

func (s *UserStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func (s *UserStore) List(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, username, role, disabled, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[User])
}

func (s *UserStore) Create(ctx context.Context, username, hash, role string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3)
		RETURNING id, username, role, disabled, created_at`,
		username, hash, role,
	).Scan(&u.ID, &u.Username, &u.Role, &u.Disabled, &u.CreatedAt)
	return u, err
}

// 활성 관리자가 0명이 되면 아무도 설정을 못 바꾸므로 그런 변경은 거부
func (s *UserStore) Update(ctx context.Context, id int64, role string, disabled bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 활성 관리자 행을 잠가 두 관리자를 동시에 중지하는 경쟁으로 0명이 되는 것을 막음
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE role = 'admin' AND NOT disabled FOR UPDATE`); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET role = $2, disabled = $3 WHERE id = $1`, id, role, disabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	var admins int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE role = 'admin' AND NOT disabled`).Scan(&admins); err != nil {
		return err
	}
	if admins == 0 {
		return ErrLastAdmin
	}
	// 중지할 때 지워 두어야 다시 사용하게 해도 예전 토큰이 되살아나지 않음
	if disabled {
		if _, err := tx.Exec(ctx,
			`DELETE FROM sessions WHERE username = (SELECT username FROM users WHERE id = $1)`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// 비밀번호가 유출돼 바꾸는 경우를 위해 keepSession(변경한 본인 세션)을 뺀 나머지 세션을 끊음
func (s *UserStore) SetPassword(ctx context.Context, username, hash, keepSession string) error {
	tag, err := s.pool.Exec(ctx, `
		WITH ended AS (DELETE FROM sessions WHERE username = $1 AND id <> $3)
		UPDATE users SET password_hash = $2 WHERE username = $1`, username, hash, keepSession)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func (s *UserStore) UsernameByID(ctx context.Context, id int64) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, id).Scan(&name)
	return name, err
}
