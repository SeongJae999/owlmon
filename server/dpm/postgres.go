package dpm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// CollectPostgres는 PostgreSQL 인스턴스에 접속하여 메트릭과 슬로우 쿼리 TOP N을 수집합니다.
// pg_stat_statements 확장이 활성화되어 있어야 슬로우 쿼리를 가져올 수 있습니다.
// 확장이 없으면 메트릭만 채우고 TopQueries는 빈 배열을 반환합니다.
func CollectPostgres(ctx context.Context, inst Instance, password string, topN int) PollResult {
	now := time.Now()
	result := PollResult{
		Metrics:    InstanceMetrics{InstanceID: inst.ID, CollectedAt: now},
		TopQueries: []QueryStat{},
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable&connect_timeout=5",
		inst.Username, password, inst.Host, inst.Port, inst.Database)

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(connCtx, dsn)
	if err != nil {
		result.Metrics.Error = fmt.Sprintf("연결 실패: %v", err)
		return result
	}
	defer conn.Close(context.Background())

	// 1. 커넥션 상태
	rows, err := conn.Query(connCtx, `
		SELECT state, COUNT(*)::int
		  FROM pg_stat_activity
		 WHERE datname = $1
		 GROUP BY state`, inst.Database)
	if err == nil {
		for rows.Next() {
			var state *string
			var count int
			if err := rows.Scan(&state, &count); err == nil && state != nil {
				switch *state {
				case "active":
					result.Metrics.ConnectionsActive = count
				case "idle":
					result.Metrics.ConnectionsIdle = count
				}
			}
		}
		rows.Close()
	}

	// 2. max_connections
	if err := conn.QueryRow(connCtx, `SHOW max_connections`).Scan(new(string)); err == nil {
		var maxConns int
		_ = conn.QueryRow(connCtx, `SELECT setting::int FROM pg_settings WHERE name='max_connections'`).Scan(&maxConns)
		result.Metrics.ConnectionsMax = maxConns
	}

	// 3. 캐시 히트율
	_ = conn.QueryRow(connCtx, `
		SELECT COALESCE(
		         sum(blks_hit)::float / NULLIF(sum(blks_hit + blks_read), 0),
		         0)
		  FROM pg_stat_database WHERE datname = $1`, inst.Database).Scan(&result.Metrics.CacheHitRatio)

	// 4. DB 사이즈
	_ = conn.QueryRow(connCtx, `SELECT pg_database_size($1)`, inst.Database).Scan(&result.Metrics.DBSizeBytes)

	// 5. 슬로우 쿼리 TOP N (pg_stat_statements 필요)
	if topN <= 0 {
		topN = 20
	}
	queryRows, err := conn.Query(connCtx, `
		SELECT queryid::text,
		       LEFT(query, 500) AS query_text,
		       calls,
		       total_exec_time,
		       mean_exec_time,
		       max_exec_time,
		       rows
		  FROM pg_stat_statements
		 ORDER BY mean_exec_time DESC
		 LIMIT $1`, topN)
	if err != nil {
		// pg_stat_statements 미설치는 치명적 오류 아님 — Notice로 표시 (Error 아님)
		if strings.Contains(err.Error(), "does not exist") {
			result.Metrics.Notice = "pg_stat_statements_missing"
		} else {
			result.Metrics.Notice = fmt.Sprintf("슬로우 쿼리 수집 불가: %v", err)
		}
		return result
	}
	defer queryRows.Close()

	for queryRows.Next() {
		q := QueryStat{InstanceID: inst.ID, CollectedAt: now}
		if err := queryRows.Scan(&q.QueryID, &q.QueryText, &q.Calls, &q.TotalTimeMs, &q.MeanTimeMs, &q.MaxTimeMs, &q.RowsReturned); err == nil {
			result.TopQueries = append(result.TopQueries, q)
		}
	}

	return result
}
