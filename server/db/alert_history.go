package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AlertRecord struct {
	ID       int64     `json:"id"`
	SentAt   time.Time `json:"sent_at"`
	Host     string    `json:"host"`
	Category string    `json:"category"`
	Severity string    `json:"severity"`
	Subject  string    `json:"subject"`
	Body     string    `json:"body"`
}

type AlertHistoryStore struct {
	pool *pgxpool.Pool
}

func NewAlertHistoryStore(pool *pgxpool.Pool) *AlertHistoryStore {
	return &AlertHistoryStore{pool: pool}
}

// Save는 알림 발송 기록을 저장합니다.
func (s *AlertHistoryStore) Save(ctx context.Context, r AlertRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO alert_history (host, category, severity, subject, body)
		 VALUES ($1, $2, $3, $4, $5)`,
		r.Host, r.Category, r.Severity, r.Subject, r.Body,
	)
	return err
}

// List는 최근 알림 히스토리를 반환합니다.
func (s *AlertHistoryStore) List(ctx context.Context, limit int) ([]AlertRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, sent_at, host, category, severity, subject, body
		 FROM alert_history
		 ORDER BY sent_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AlertRecord
	for rows.Next() {
		var r AlertRecord
		if err := rows.Scan(&r.ID, &r.SentAt, &r.Host, &r.Category, &r.Severity, &r.Subject, &r.Body); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
