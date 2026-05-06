package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LogRecord는 수집된 로그 한 줄입니다.
type LogRecord struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Host      string    `json:"host"`
	Source    string    `json:"source"`
	FilePath  string    `json:"file_path"`
	Line      string    `json:"line"`
	Level     string    `json:"level"`
	CreatedAt time.Time `json:"created_at"`
}

// LogSearchParams는 로그 검색 조건입니다.
type LogSearchParams struct {
	Host   string
	Source string
	Level  string
	Query  string // ILIKE 검색
	From   time.Time
	To     time.Time
	Limit  int
	Offset int
}

// LogSearchResult는 로그 검색 결과입니다.
type LogSearchResult struct {
	Records []LogRecord `json:"records"`
	Total   int         `json:"total"`
}

// LogSource는 호스트별 로그 소스입니다.
type LogSource struct {
	Host   string `json:"host"`
	Source string `json:"source"`
}

// LogStore는 PostgreSQL 기반 로그 저장소입니다.
type LogStore struct {
	pool *pgxpool.Pool
}

func NewLogStore(pool *pgxpool.Pool) *LogStore {
	return &LogStore{pool: pool}
}

// Ingest는 로그 레코드를 배치 삽입합니다.
func (s *LogStore) Ingest(ctx context.Context, records []LogRecord) error {
	if len(records) == 0 {
		return nil
	}
	// 배치당 최대 1000건 제한
	if len(records) > 1000 {
		records = records[:1000]
	}

	entries := make([][]any, len(records))
	for i, r := range records {
		entries[i] = []any{r.Timestamp, r.Host, r.Source, r.FilePath, r.Line, r.Level}
	}

	_, err := s.pool.CopyFrom(
		ctx,
		pgx.Identifier{"logs"},
		[]string{"timestamp", "host", "source", "file_path", "line", "level"},
		pgx.CopyFromRows(entries),
	)
	return err
}

// Search는 로그를 검색합니다.
func (s *LogStore) Search(ctx context.Context, p LogSearchParams) (*LogSearchResult, error) {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 100
	}

	var where []string
	var args []interface{}
	argN := 1

	if p.Host != "" {
		where = append(where, fmt.Sprintf("host=$%d", argN))
		args = append(args, p.Host)
		argN++
	}
	if p.Source != "" {
		where = append(where, fmt.Sprintf("source=$%d", argN))
		args = append(args, p.Source)
		argN++
	}
	if p.Level != "" {
		where = append(where, fmt.Sprintf("level=$%d", argN))
		args = append(args, p.Level)
		argN++
	}
	if p.Query != "" {
		where = append(where, fmt.Sprintf("line ILIKE $%d", argN))
		args = append(args, "%"+p.Query+"%")
		argN++
	}
	if !p.From.IsZero() {
		where = append(where, fmt.Sprintf("timestamp>=$%d", argN))
		args = append(args, p.From)
		argN++
	}
	if !p.To.IsZero() {
		where = append(where, fmt.Sprintf("timestamp<=$%d", argN))
		args = append(args, p.To)
		argN++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// 총 건수
	var total int
	countQuery := "SELECT COUNT(*) FROM logs " + whereClause
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	// 데이터 조회
	dataQuery := fmt.Sprintf(
		"SELECT id, timestamp, host, source, file_path, line, level, created_at FROM logs %s ORDER BY timestamp DESC LIMIT $%d OFFSET $%d",
		whereClause, argN, argN+1,
	)
	dataArgs := append(args, p.Limit, p.Offset)

	rows, err := s.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []LogRecord
	for rows.Next() {
		var r LogRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Host, &r.Source, &r.FilePath, &r.Line, &r.Level, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	if records == nil {
		records = []LogRecord{}
	}
	return &LogSearchResult{Records: records, Total: total}, nil
}

// Sources는 호스트별 로그 소스 목록을 반환합니다.
func (s *LogStore) Sources(ctx context.Context) ([]LogSource, error) {
	rows, err := s.pool.Query(ctx, "SELECT DISTINCT host, source FROM logs ORDER BY host, source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []LogSource
	for rows.Next() {
		var src LogSource
		if err := rows.Scan(&src.Host, &src.Source); err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	if sources == nil {
		sources = []LogSource{}
	}
	return sources, nil
}

// GetByID는 단일 로그 레코드를 반환합니다.
// 라벨링 UI에서 로그 상세를 보여줄 때 사용.
// 존재하지 않으면 pgx.ErrNoRows 반환.
func (s *LogStore) GetByID(ctx context.Context, id int64) (*LogRecord, error) {
	var r LogRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, timestamp, host, source, file_path, line, level, created_at
		 FROM logs WHERE id = $1`,
		id,
	).Scan(&r.ID, &r.Timestamp, &r.Host, &r.Source, &r.FilePath, &r.Line, &r.Level, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Cleanup은 retentionDays보다 오래된 로그를 삭제합니다.
func (s *LogStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	tag, err := s.pool.Exec(ctx, "DELETE FROM logs WHERE created_at < $1", cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
