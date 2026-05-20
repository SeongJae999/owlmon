package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MatchedRule은 로그가 매칭된 룰 요약.
type MatchedRule struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
}

// LogRecord는 수집된 로그 한 줄입니다.
type LogRecord struct {
	ID            int64         `json:"id"`
	Timestamp     time.Time     `json:"timestamp"`
	Host          string        `json:"host"`
	Source        string        `json:"source"`
	FilePath      string        `json:"file_path"`
	Line          string        `json:"line"`
	Level         string        `json:"level"`
	CreatedAt     time.Time     `json:"created_at"`
	MatchedRules  []MatchedRule `json:"matched_rules,omitempty"` // Search 시 채워짐 (Ingest 시엔 비움)
}

// LogSearchParams는 로그 검색 조건입니다.
type LogSearchParams struct {
	Host   string
	Source string
	Level  string
	Query  string // ILIKE 검색
	RuleID int64  // > 0이면 해당 룰에 매칭된 로그만
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

// Ingest는 로그 레코드를 배치 삽입하고 생성된 id 목록을 반환한다.
// 룰 매칭에서 log_rule_matches.log_id FK 채우는 데 사용.
func (s *LogStore) Ingest(ctx context.Context, records []LogRecord) ([]int64, error) {
	if len(records) == 0 {
		return nil, nil
	}
	// 배치당 최대 1000건 제한 (INSERT 파라미터 6000개 이내)
	if len(records) > 1000 {
		records = records[:1000]
	}

	// INSERT ... VALUES ($1,$2,...) RETURNING id
	// (CopyFrom은 RETURNING 지원 안 함 — 룰 매칭 위해 id 필요)
	var sb strings.Builder
	sb.WriteString("INSERT INTO logs (timestamp, host, source, file_path, line, level) VALUES ")
	args := make([]any, 0, len(records)*6)
	for i, r := range records {
		if i > 0 {
			sb.WriteString(",")
		}
		base := i * 6
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6)
		args = append(args, r.Timestamp, r.Host, r.Source, r.FilePath, r.Line, r.Level)
	}
	sb.WriteString(" RETURNING id")

	rows, err := s.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0, len(records))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
	// 룰 필터: 해당 룰에 매칭된 log_id만
	if p.RuleID > 0 {
		where = append(where, fmt.Sprintf("id IN (SELECT log_id FROM log_rule_matches WHERE rule_id=$%d)", argN))
		args = append(args, p.RuleID)
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

	// 데이터 조회 — 매칭 룰을 jsonb_agg로 같이 가져옴 (LEFT JOIN + GROUP BY)
	dataQuery := fmt.Sprintf(`
		SELECT l.id, l.timestamp, l.host, l.source, l.file_path, l.line, l.level, l.created_at,
		       COALESCE(
		         jsonb_agg(jsonb_build_object('id', lr.id, 'name', lr.name, 'severity', lr.severity))
		           FILTER (WHERE lr.id IS NOT NULL),
		         '[]'::jsonb
		       ) AS matched_rules
		FROM logs l
		LEFT JOIN log_rule_matches lrm ON lrm.log_id = l.id
		LEFT JOIN log_rules        lr  ON lr.id     = lrm.rule_id
		%s
		GROUP BY l.id
		ORDER BY l.timestamp DESC
		LIMIT $%d OFFSET $%d`,
		strings.Replace(whereClause, "host=", "l.host=", 1), // host 컬럼 alias 명시
		argN, argN+1,
	)
	// 단순 alias 치환 위 1회 — host/source/level/timestamp/line/id 모두 명시 필요
	dataQuery = strings.NewReplacer(
		"WHERE host=", "WHERE l.host=",
		" AND host=", " AND l.host=",
		"WHERE source=", "WHERE l.source=",
		" AND source=", " AND l.source=",
		"WHERE level=", "WHERE l.level=",
		" AND level=", " AND l.level=",
		"WHERE line ILIKE", "WHERE l.line ILIKE",
		" AND line ILIKE", " AND l.line ILIKE",
		"WHERE timestamp", "WHERE l.timestamp",
		" AND timestamp", " AND l.timestamp",
		"WHERE id IN", "WHERE l.id IN",
		" AND id IN", " AND l.id IN",
	).Replace(dataQuery)

	dataArgs := append(args, p.Limit, p.Offset)

	rows, err := s.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []LogRecord
	for rows.Next() {
		var r LogRecord
		var matchedRulesJSON []byte
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Host, &r.Source, &r.FilePath, &r.Line, &r.Level, &r.CreatedAt, &matchedRulesJSON); err != nil {
			return nil, err
		}
		if len(matchedRulesJSON) > 0 {
			_ = json.Unmarshal(matchedRulesJSON, &r.MatchedRules)
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
