package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/synthetic"
)

// SyntheticStore는 PostgreSQL 기반 Synthetic 모니터/결과 저장소입니다.
type SyntheticStore struct {
	pool *pgxpool.Pool
}

func NewSyntheticStore(pool *pgxpool.Pool) *SyntheticStore {
	return &SyntheticStore{pool: pool}
}

// List는 등록된 모니터를 모두 반환합니다.
func (s *SyntheticStore) List(ctx context.Context) ([]synthetic.Monitor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, url, method, expected_status, expected_keyword,
		       interval_seconds, timeout_seconds, enabled
		  FROM synthetic_monitors
		 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []synthetic.Monitor
	for rows.Next() {
		var m synthetic.Monitor
		if err := rows.Scan(
			&m.ID, &m.Name, &m.URL, &m.Method, &m.ExpectedStatus, &m.ExpectedKeyword,
			&m.IntervalSeconds, &m.TimeoutSeconds, &m.Enabled,
		); err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, nil
}

// Add는 새 모니터를 등록합니다.
func (s *SyntheticStore) Add(ctx context.Context, m synthetic.Monitor) (synthetic.Monitor, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO synthetic_monitors
			(name, url, method, expected_status, expected_keyword, interval_seconds, timeout_seconds, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		m.Name, m.URL, m.Method, m.ExpectedStatus, m.ExpectedKeyword,
		m.IntervalSeconds, m.TimeoutSeconds, m.Enabled,
	).Scan(&m.ID)
	return m, err
}

// Update는 기존 모니터를 수정합니다.
func (s *SyntheticStore) Update(ctx context.Context, m synthetic.Monitor) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE synthetic_monitors
		   SET name=$2, url=$3, method=$4, expected_status=$5, expected_keyword=$6,
		       interval_seconds=$7, timeout_seconds=$8, enabled=$9
		 WHERE id=$1`,
		m.ID, m.Name, m.URL, m.Method, m.ExpectedStatus, m.ExpectedKeyword,
		m.IntervalSeconds, m.TimeoutSeconds, m.Enabled,
	)
	return err
}

// Delete는 모니터를 삭제합니다 (결과는 CASCADE).
func (s *SyntheticStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM synthetic_monitors WHERE id=$1`, id)
	return err
}

// SaveResult는 체크 결과 1건을 저장합니다.
func (s *SyntheticStore) SaveResult(ctx context.Context, r synthetic.Result) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO synthetic_results
			(monitor_id, success, status_code, response_time_ms, error, checked_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		r.MonitorID, r.Success, r.StatusCode, r.ResponseTimeMs, r.Error, r.CheckedAt,
	)
	return err
}

// RecentResults는 특정 모니터의 최근 N건 결과를 반환합니다 (최신순).
func (s *SyntheticStore) RecentResults(ctx context.Context, monitorID int64, limit int) ([]synthetic.Result, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT monitor_id, success, status_code, response_time_ms, error, checked_at
		  FROM synthetic_results
		 WHERE monitor_id=$1
		 ORDER BY checked_at DESC
		 LIMIT $2`, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []synthetic.Result
	for rows.Next() {
		var r synthetic.Result
		if err := rows.Scan(&r.MonitorID, &r.Success, &r.StatusCode, &r.ResponseTimeMs, &r.Error, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// Stats는 특정 모니터의 최근 기간 통계를 계산합니다.
type SyntheticStats struct {
	MonitorID    int64   `json:"monitor_id"`
	UptimePct    float64 `json:"uptime_pct"`     // 성공률 %
	AvgLatencyMs float64 `json:"avg_latency_ms"` // 평균 응답시간 (성공만)
	P95LatencyMs int     `json:"p95_latency_ms"` // 95퍼센타일 응답시간 (성공만)
	TotalChecks  int     `json:"total_checks"`
	FailedChecks int     `json:"failed_checks"`
}

// StatsAll은 모든 모니터의 최근 N시간 통계를 한 번에 반환합니다. 결과가 없는 모니터는 빠집니다.
func (s *SyntheticStore) StatsAll(ctx context.Context, hours int) (map[int64]SyntheticStats, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	// 모니터마다 (monitor_id, checked_at) 인덱스로 최근 범위만 읽음 — checked_at만으로 거르면 이력 전체를 훑음
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, x.total, x.failed, x.avg, x.p95
		  FROM synthetic_monitors m
		 CROSS JOIN LATERAL (
			SELECT COUNT(*) AS total,
			       COUNT(*) FILTER (WHERE NOT success) AS failed,
			       COALESCE(AVG(response_time_ms) FILTER (WHERE success), 0) AS avg,
			       COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY response_time_ms) FILTER (WHERE success), 0) AS p95
			  FROM synthetic_results r
			 WHERE r.monitor_id = m.id AND r.checked_at >= $1
		 ) x
		 WHERE x.total > 0`,
		since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]SyntheticStats{}
	for rows.Next() {
		var st SyntheticStats
		if err := rows.Scan(&st.MonitorID, &st.TotalChecks, &st.FailedChecks, &st.AvgLatencyMs, &st.P95LatencyMs); err != nil {
			return nil, err
		}
		if st.TotalChecks > 0 {
			st.UptimePct = float64(st.TotalChecks-st.FailedChecks) / float64(st.TotalChecks) * 100
		}
		out[st.MonitorID] = st
	}
	return out, rows.Err()
}

// Cleanup은 N일 이전 결과를 삭제합니다.
func (s *SyntheticStore) Cleanup(ctx context.Context, days int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	return deleteBatched(ctx, s.pool, `DELETE FROM synthetic_results WHERE ctid IN (SELECT ctid FROM synthetic_results WHERE checked_at < $1 LIMIT 10000)`, cutoff)
}

// 정리 없이 쌓인 이력을 한 문장으로 지우면 긴 트랜잭션·WAL 폭증이 생김 — 작게 나눠 지움
func deleteBatched(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (int64, error) {
	var total int64
	for {
		tag, err := pool.Exec(ctx, sql, args...)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() == 0 {
			return total, nil
		}
	}
}
