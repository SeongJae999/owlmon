package db

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

// init.sql은 새 DB에서만 돌기 때문에 기존 운영 DB에도 생기도록 기동 시 추가
func (s *AlertConfigStore) EnsureColumns(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `ALTER TABLE alert_config ADD COLUMN IF NOT EXISTS channels JSONB NOT NULL DEFAULT '{}'`)
	return err
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
	// 따로 읽음 — 컬럼 추가가 실패한 DB에서도 수신자·임계값은 그대로 읽혀야 함 (한 번에 읽으면 전부 기본값이 됨)
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT channels FROM alert_config WHERE id = 1`).Scan(&raw); err == nil {
		if err := json.Unmarshal(raw, &cfg.Channels); err != nil {
			log.Printf("알림 경로 설정 읽기 실패: %v", err)
		}
	}
	return cfg
}

func (s *AlertConfigStore) Set(cfg alert.AlertConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	channels, err := json.Marshal(cfg.Channels)
	if err != nil {
		return err
	}
	// 한 트랜잭션 — 일부만 저장된 채 실패 응답이 나가지 않게
	err = s.save(ctx, cfg, channels)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42703" {
		// channels 칸 추가(EnsureColumns)가 실패한 DB — 임계값·수신자는 저장하고 경로 설정만 못 했다고 알림
		if err := s.save(ctx, cfg, nil); err != nil {
			return err
		}
		return alert.ErrChannelsNotSaved
	}
	return err
}

// SetKeepChannels는 웹훅·문자 설정 칸은 그대로 두고 나머지만 저장
func (s *AlertConfigStore) SetKeepChannels(cfg alert.AlertConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.save(ctx, cfg, nil)
}

func (s *AlertConfigStore) save(ctx context.Context, cfg alert.AlertConfig, channels []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx,
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
	if err != nil {
		return err
	}
	if channels != nil {
		if _, err := tx.Exec(ctx, `UPDATE alert_config SET channels = $1 WHERE id = 1`, channels); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
