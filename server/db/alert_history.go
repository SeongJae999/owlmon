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
	return s.Search(ctx, AlertHistoryFilter{Limit: limit})
}

// AlertHistoryFilter — 알림 히스토리 검색 필터 (시간 범위 / 심각도 / 호스트)
type AlertHistoryFilter struct {
	From     time.Time
	To       time.Time
	Severity string
	Host     string
	Limit    int
}

// Search는 필터 조건으로 알림 히스토리를 조회합니다.
func (s *AlertHistoryStore) Search(ctx context.Context, f AlertHistoryFilter) ([]AlertRecord, error) {
	if f.Limit <= 0 || f.Limit > 10000 {
		f.Limit = 500
	}

	q := `SELECT id, sent_at, host, category, severity, subject, body FROM alert_history WHERE 1=1`
	args := []interface{}{}
	argN := 1

	if !f.From.IsZero() {
		q += " AND sent_at >= $" + itoa(argN)
		args = append(args, f.From)
		argN++
	}
	if !f.To.IsZero() {
		q += " AND sent_at <= $" + itoa(argN)
		args = append(args, f.To)
		argN++
	}
	if f.Severity != "" {
		q += " AND severity = $" + itoa(argN)
		args = append(args, f.Severity)
		argN++
	}
	if f.Host != "" {
		q += " AND host = $" + itoa(argN)
		args = append(args, f.Host)
		argN++
	}
	q += " ORDER BY sent_at DESC LIMIT $" + itoa(argN)
	args = append(args, f.Limit)

	rows, err := s.pool.Query(ctx, q, args...)
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

// itoa — strconv 의존 없이 정수 → 문자열
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
