package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HistorySaver는 alert.HistorySaver 인터페이스를 구현합니다.
type HistorySaver struct {
	store *AlertHistoryStore
}

func NewHistorySaver(pool *pgxpool.Pool) *HistorySaver {
	return &HistorySaver{store: NewAlertHistoryStore(pool)}
}

func (h *HistorySaver) Save(ctx context.Context, host, category, severity, subject, body string) error {
	return h.store.Save(ctx, AlertRecord{
		Host:     host,
		Category: category,
		Severity: severity,
		Subject:  subject,
		Body:     body,
	})
}
