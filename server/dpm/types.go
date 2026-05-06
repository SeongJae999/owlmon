// Package dpm은 외부 데이터베이스 인스턴스에 접속하여 슬로우 쿼리/커넥션 통계를 수집합니다.
package dpm

import "time"

// Instance는 모니터링 대상 DB 정의입니다.
type Instance struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	DBType          string `json:"db_type"` // "postgres" (Phase 2 한정)
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Database        string `json:"database"`
	PollIntervalSec int    `json:"poll_interval_sec"`
	Enabled         bool   `json:"enabled"`

	// 평문 비밀번호는 API 입력 시에만 채워지고, 저장 시에는 PasswordEnc 사용
	Password    string `json:"password,omitempty"`
	PasswordEnc string `json:"-"` // base64(AES-GCM(password))
}

// QueryStat는 슬로우 쿼리 1건의 누적 통계입니다 (pg_stat_statements 기준).
type QueryStat struct {
	InstanceID   int64     `json:"instance_id"`
	QueryID      string    `json:"query_id"`
	QueryText    string    `json:"query_text"`
	Calls        int64     `json:"calls"`
	TotalTimeMs  float64   `json:"total_time_ms"`
	MeanTimeMs   float64   `json:"mean_time_ms"`
	MaxTimeMs    float64   `json:"max_time_ms"`
	RowsReturned int64     `json:"rows_returned"`
	CollectedAt  time.Time `json:"collected_at"`
}

// InstanceMetrics는 인스턴스 단위의 즉시 메트릭입니다.
type InstanceMetrics struct {
	InstanceID        int64     `json:"instance_id"`
	ConnectionsActive int       `json:"connections_active"`
	ConnectionsIdle   int       `json:"connections_idle"`
	ConnectionsMax    int       `json:"connections_max"`
	CacheHitRatio     float64   `json:"cache_hit_ratio"` // 0.0 ~ 1.0
	DBSizeBytes       int64     `json:"db_size_bytes"`
	Error             string    `json:"error,omitempty"`
	CollectedAt       time.Time `json:"collected_at"`
}

// PollResult는 1회 폴링의 전체 결과입니다.
type PollResult struct {
	Metrics  InstanceMetrics
	TopQueries []QueryStat
}
