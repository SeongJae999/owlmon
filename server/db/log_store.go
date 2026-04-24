package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LogLevel은 로그 심각도입니다.
// 값은 OpenTelemetry SeverityNumber 스펙과 일치 (1/5/9/13/17/21).
// 참조: https://opentelemetry.io/docs/specs/otel/logs/data-model/#field-severitynumber
type LogLevel int16

const (
	LogTrace LogLevel = 1
	LogDebug LogLevel = 5
	LogInfo  LogLevel = 9
	LogWarn  LogLevel = 13
	LogError LogLevel = 17
	LogFatal LogLevel = 21
)

// String은 레벨을 사람이 읽기 쉬운 문자열로 반환합니다.
func (l LogLevel) String() string {
	switch {
	case l >= LogFatal:
		return "fatal"
	case l >= LogError:
		return "error"
	case l >= LogWarn:
		return "warn"
	case l >= LogInfo:
		return "info"
	case l >= LogDebug:
		return "debug"
	default:
		return "trace"
	}
}

// LogRecord는 단일 로그 레코드입니다.
// 에이전트가 수집한 로그가 HTTP 핸들러 → ingest 워커를 거쳐
// 이 타입으로 변환된 뒤 InsertBatch로 저장됩니다.
type LogRecord struct {
	ID         int64          `json:"id"`
	HostName   string         `json:"host_name"`
	Source     string         `json:"source"` // 예: "journald", "file:/var/log/nginx/error.log", "winevent:System"
	Timestamp  time.Time      `json:"timestamp"`
	Severity   LogLevel       `json:"severity"`
	Message    string         `json:"message"`
	TemplateID *string        `json:"template_id,omitempty"` // Phase 1에서 채움
	Attributes map[string]any `json:"attributes,omitempty"`
}

// LogStore는 logs 파티션 테이블에 대한 저장소입니다.
type LogStore struct {
	pool *pgxpool.Pool
}

func NewLogStore(pool *pgxpool.Pool) *LogStore {
	return &LogStore{pool: pool}
}

// InsertBatch는 PostgreSQL COPY 프로토콜로 로그 배치를 대량 저장합니다.
// 초당 5,000건+ 처리를 위한 핵심 경로.
// 반환값: 실제 삽입된 행 수.
func (s *LogStore) InsertBatch(ctx context.Context, records []LogRecord) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}

	rows := make([][]any, len(records))
	for i, r := range records {
		rows[i] = []any{
			r.HostName,
			r.Source,
			r.Timestamp,
			int16(r.Severity),
			r.Message,
			r.TemplateID, // nil이면 NULL로 저장
			r.Attributes, // nil이면 NULL로 저장
		}
	}

	return s.pool.CopyFrom(ctx,
		pgx.Identifier{"logs"},
		[]string{"host_name", "source", "timestamp", "severity", "message", "template_id", "attributes"},
		pgx.CopyFromRows(rows),
	)
}

// CountSince는 주어진 시각 이후 저장된 로그 수를 반환합니다 (헬스체크/테스트용).
func (s *LogStore) CountSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM logs WHERE timestamp >= $1`,
		since,
	).Scan(&count)
	return count, err
}
