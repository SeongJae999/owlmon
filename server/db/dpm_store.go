package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/dpm"
)

// DPMStore는 DPM 인스턴스/메트릭/슬로우쿼리 저장소입니다.
type DPMStore struct {
	pool *pgxpool.Pool
}

func NewDPMStore(pool *pgxpool.Pool) *DPMStore {
	return &DPMStore{pool: pool}
}

// List는 등록된 모든 인스턴스를 반환합니다 (PasswordEnc 포함).
func (s *DPMStore) List(ctx context.Context) ([]dpm.Instance, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, db_type, host, port, username, password_enc,
		       database, poll_interval_sec, enabled
		  FROM dpm_instances
		 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dpm.Instance
	for rows.Next() {
		var i dpm.Instance
		if err := rows.Scan(&i.ID, &i.Name, &i.DBType, &i.Host, &i.Port, &i.Username,
			&i.PasswordEnc, &i.Database, &i.PollIntervalSec, &i.Enabled); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// Add는 새 인스턴스를 등록합니다.
func (s *DPMStore) Add(ctx context.Context, i dpm.Instance) (dpm.Instance, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dpm_instances
			(name, db_type, host, port, username, password_enc, database, poll_interval_sec, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`,
		i.Name, i.DBType, i.Host, i.Port, i.Username, i.PasswordEnc,
		i.Database, i.PollIntervalSec, i.Enabled,
	).Scan(&i.ID)
	return i, err
}

// Delete는 인스턴스를 삭제합니다 (메트릭/쿼리 통계는 CASCADE).
func (s *DPMStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM dpm_instances WHERE id=$1`, id)
	return err
}

// SaveMetrics는 인스턴스 메트릭 1건을 저장합니다.
func (s *DPMStore) SaveMetrics(ctx context.Context, m dpm.InstanceMetrics) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dpm_instance_metrics
			(instance_id, connections_active, connections_idle, connections_max,
			 cache_hit_ratio, db_size_bytes, error, collected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		m.InstanceID, m.ConnectionsActive, m.ConnectionsIdle, m.ConnectionsMax,
		m.CacheHitRatio, m.DBSizeBytes, m.Error, m.CollectedAt,
	)
	return err
}

// SaveQueryStats는 슬로우 쿼리 통계 N건을 저장합니다.
func (s *DPMStore) SaveQueryStats(ctx context.Context, stats []dpm.QueryStat) error {
	if len(stats) == 0 {
		return nil
	}
	batch := make([][]interface{}, len(stats))
	for i, q := range stats {
		batch[i] = []interface{}{q.InstanceID, q.QueryID, q.QueryText, q.Calls,
			q.TotalTimeMs, q.MeanTimeMs, q.MaxTimeMs, q.RowsReturned, q.CollectedAt}
	}
	for _, args := range batch {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO dpm_query_stats
				(instance_id, query_id, query_text, calls,
				 total_time_ms, mean_time_ms, max_time_ms, rows_returned, collected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

// LatestQueryStats는 특정 인스턴스의 가장 최근 폴링 시점의 슬로우 쿼리들을 반환합니다.
func (s *DPMStore) LatestQueryStats(ctx context.Context, instanceID int64) ([]dpm.QueryStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT instance_id, query_id, query_text, calls, total_time_ms, mean_time_ms, max_time_ms, rows_returned, collected_at
		  FROM dpm_query_stats
		 WHERE instance_id = $1
		   AND collected_at = (SELECT MAX(collected_at) FROM dpm_query_stats WHERE instance_id = $1)
		 ORDER BY mean_time_ms DESC`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dpm.QueryStat
	for rows.Next() {
		var q dpm.QueryStat
		if err := rows.Scan(&q.InstanceID, &q.QueryID, &q.QueryText, &q.Calls,
			&q.TotalTimeMs, &q.MeanTimeMs, &q.MaxTimeMs, &q.RowsReturned, &q.CollectedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

// MetricsHistory는 특정 인스턴스의 최근 N시간 메트릭을 시간순으로 반환합니다 (그래프용).
func (s *DPMStore) MetricsHistory(ctx context.Context, instanceID int64, hours int) ([]dpm.InstanceMetrics, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := s.pool.Query(ctx, `
		SELECT instance_id, connections_active, connections_idle, connections_max,
		       cache_hit_ratio, db_size_bytes, error, collected_at
		  FROM dpm_instance_metrics
		 WHERE instance_id = $1 AND collected_at >= $2
		 ORDER BY collected_at`, instanceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dpm.InstanceMetrics
	for rows.Next() {
		var m dpm.InstanceMetrics
		if err := rows.Scan(&m.InstanceID, &m.ConnectionsActive, &m.ConnectionsIdle, &m.ConnectionsMax,
			&m.CacheHitRatio, &m.DBSizeBytes, &m.Error, &m.CollectedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// Cleanup은 N일 이전의 메트릭/쿼리 통계를 삭제합니다.
func (s *DPMStore) Cleanup(ctx context.Context, days int) error {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	if _, err := s.pool.Exec(ctx, `DELETE FROM dpm_instance_metrics WHERE collected_at < $1`, cutoff); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM dpm_query_stats WHERE collected_at < $1`, cutoff); err != nil {
		return err
	}
	return nil
}
