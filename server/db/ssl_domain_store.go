package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/ssl"
)

// SSLDomain은 SSL 인증서 모니터링 대상 도메인입니다.
type SSLDomain struct {
	ID        int64     `json:"id"`
	Domain    string    `json:"domain"`
	Port      int       `json:"port"`
	Memo      string    `json:"memo"`
	CreatedAt time.Time `json:"created_at"`
}

// SSLDomainStore는 PostgreSQL 기반 SSL 도메인 저장소입니다.
type SSLDomainStore struct {
	pool *pgxpool.Pool
}

func NewSSLDomainStore(pool *pgxpool.Pool) *SSLDomainStore {
	return &SSLDomainStore{pool: pool}
}

func (s *SSLDomainStore) List(ctx context.Context) ([]SSLDomain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, domain, port, memo, created_at FROM ssl_domains ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []SSLDomain
	for rows.Next() {
		var d SSLDomain
		if err := rows.Scan(&d.ID, &d.Domain, &d.Port, &d.Memo, &d.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, nil
}

func (s *SSLDomainStore) Add(ctx context.Context, d SSLDomain) (SSLDomain, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO ssl_domains (domain, port, memo) VALUES ($1, $2, $3) RETURNING id, created_at`,
		d.Domain, d.Port, d.Memo,
	).Scan(&d.ID, &d.CreatedAt)
	return d, err
}

func (s *SSLDomainStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ssl_domains WHERE id=$1`, id)
	return err
}

// UpdateMemo는 도메인의 메모만 업데이트합니다. (도메인/포트는 식별자라 변경 불가)
func (s *SSLDomainStore) UpdateMemo(ctx context.Context, id int64, memo string) error {
	_, err := s.pool.Exec(ctx, `UPDATE ssl_domains SET memo=$2 WHERE id=$1`, id, memo)
	return err
}

// ListEntries는 alert.SSLDomainLister 인터페이스를 구현합니다.
func (s *SSLDomainStore) ListEntries(ctx context.Context) ([]ssl.DomainEntry, error) {
	domains, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]ssl.DomainEntry, len(domains))
	for i, d := range domains {
		entries[i] = ssl.DomainEntry{ID: d.ID, Domain: d.Domain, Port: d.Port}
	}
	return entries, nil
}
