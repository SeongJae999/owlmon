package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/alert"
)

// AlertConfigStore는 PostgreSQL에서 알림 설정을 읽고 씁니다.
type AlertConfigStore struct {
	pool *pgxpool.Pool
}

func NewAlertConfigStore(pool *pgxpool.Pool) *AlertConfigStore {
	return &AlertConfigStore{pool: pool}
}

func (s *AlertConfigStore) Get() alert.AlertConfig {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := defaultAlertConfig()
	row := s.pool.QueryRow(ctx,
		`SELECT enabled, recipients, cpu_threshold, mem_threshold, disk_warn, disk_crit
		 FROM alert_config WHERE id = 1`,
	)
	_ = row.Scan(
		&cfg.Enabled,
		&cfg.Recipients,
		&cfg.CPUThreshold,
		&cfg.MemThreshold,
		&cfg.DiskWarn,
		&cfg.DiskCrit,
	)
	return cfg
}

func (s *AlertConfigStore) Set(cfg alert.AlertConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO alert_config (id, enabled, recipients, cpu_threshold, mem_threshold, disk_warn, disk_crit, updated_at)
		 VALUES (1, $1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT (id) DO UPDATE SET
		   enabled = EXCLUDED.enabled,
		   recipients = EXCLUDED.recipients,
		   cpu_threshold = EXCLUDED.cpu_threshold,
		   mem_threshold = EXCLUDED.mem_threshold,
		   disk_warn = EXCLUDED.disk_warn,
		   disk_crit = EXCLUDED.disk_crit,
		   updated_at = NOW()`,
		cfg.Enabled, cfg.Recipients, cfg.CPUThreshold, cfg.MemThreshold, cfg.DiskWarn, cfg.DiskCrit,
	)
	return err
}

func defaultAlertConfig() alert.AlertConfig {
	return alert.AlertConfig{
		Enabled:      true,
		Recipients:   []string{},
		CPUThreshold: 90,
		MemThreshold: 95,
		DiskWarn:     85,
		DiskCrit:     90,
	}
}
