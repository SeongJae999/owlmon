package loginsight

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store — log_insights 테이블 CRUD + log_template_cache 관리.
// OWLmon의 기존 db 패턴(pgx/v5 + pgxpool)을 따른다.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore — pool이 nil이면 nil 반환 (DB 비활성 환경 안전).
func NewStore(pool *pgxpool.Pool) *Store {
	if pool == nil {
		return nil
	}
	return &Store{pool: pool}
}

// Save — Insight 한 건 저장 + 템플릿 캐시 갱신 (단일 트랜잭션).
// 성공 시 ins.ID가 채워진다.
func (s *Store) Save(ctx context.Context, ins *Insight) error {
	if s == nil || s.pool == nil {
		return errors.New("Store: pool 미설정")
	}
	if ins == nil {
		return errors.New("insight is nil")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO log_insights
			(template_hash, sample_log_ids, host_name, severity, category,
			 summary_ko, root_cause_ko, action_ko, needs_human,
			 model_name, prompt_tokens, output_tokens, latency_ms, created_at, incident_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id`,
		ins.TemplateHash,
		ins.SampleLogIDs, // pgx가 []int64 → BIGINT[] 자동 변환
		nullStr(ins.HostName),
		string(ins.Severity),
		string(ins.Category),
		ins.SummaryKo,
		nullStr(ins.RootCauseKo),
		nullStr(ins.ActionKo),
		ins.NeedsHuman,
		ins.ModelName,
		nullInt(ins.PromptTokens),
		nullInt(ins.OutputTokens),
		nullInt(ins.LatencyMs),
		ins.CreatedAt,
		nullStr(ins.IncidentKey),
	).Scan(&ins.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO log_template_cache (template_hash, last_insight_id, hit_count, last_seen_at)
		VALUES ($1, $2, 1, now())
		ON CONFLICT (template_hash) DO UPDATE
		SET last_insight_id = EXCLUDED.last_insight_id,
		    hit_count       = log_template_cache.hit_count + 1,
		    last_seen_at    = now()`,
		ins.TemplateHash, ins.ID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Filter — List 조회 옵션.
type Filter struct {
	HostName string    // 빈 문자열이면 무시
	Severity string    // 빈 문자열이면 무시 (예: "critical", "high")
	From     time.Time // zero면 무시
	To       time.Time // zero면 무시
	Limit    int       // 0 또는 500 초과면 50으로 클램프
}

// List — 필터에 맞는 인사이트를 최신순으로 반환.
func (s *Store) List(ctx context.Context, f Filter) ([]Insight, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("Store: pool 미설정")
	}
	q, args := buildListQuery(f)
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Insight
	for rows.Next() {
		ins, err := scanInsight(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ins)
	}
	return out, rows.Err()
}

// ByTemplate — (템플릿, 호스트)의 maxAge 내 최신 인사이트. host가 ""이면 호스트 무관.
// 캐시 hit이 없거나 만료되었으면 (nil, nil).
// Worker가 LLM 호출 직전에 체크해서 중복 호출을 방지.
func (s *Store) ByTemplate(ctx context.Context, hash, host string, maxAge time.Duration) (*Insight, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("Store: pool 미설정")
	}
	if hash == "" {
		return nil, errors.New("hash empty")
	}
	cutoff := time.Now().Add(-maxAge)

	// 만료 기준은 분석 시각 — 재발 시각(last_seen_at) 기준이면 반복 패턴이 영영 재분석되지 않는다
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM log_insights
		WHERE template_hash = $1 AND ($2 = '' OR host_name = $2) AND created_at >= $3
		ORDER BY created_at DESC LIMIT 1`,
		hash, host, cutoff,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return s.getByID(ctx, id)
}

// History — 이 호스트에서 이 패턴의 첫 분석 시각과 분석 횟수 (이력 없으면 n=0).
func (s *Store) History(ctx context.Context, hash, host string) (time.Time, int, error) {
	if s == nil || s.pool == nil {
		return time.Time{}, 0, errors.New("Store: pool 미설정")
	}
	var first *time.Time
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT min(created_at), count(*) FROM log_insights
		WHERE template_hash = $1 AND host_name = $2`,
		hash, host,
	).Scan(&first, &n)
	if err != nil || first == nil {
		return time.Time{}, 0, err
	}
	return *first, n, nil
}

// TouchTemplate — 같은 패턴이 또 등장했지만 LLM 호출은 건너뛸 때
// hit_count + last_seen_at만 갱신.
func (s *Store) TouchTemplate(ctx context.Context, hash string) error {
	if s == nil || s.pool == nil {
		return errors.New("Store: pool 미설정")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE log_template_cache
		SET hit_count = hit_count + 1, last_seen_at = now()
		WHERE template_hash = $1`, hash)
	return err
}

// === 내부 헬퍼 ===

// buildListQuery — Filter → SQL + args. 단위 테스트를 위해 분리.
func buildListQuery(f Filter) (string, []any) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	q := `SELECT id, template_hash, sample_log_ids, COALESCE(host_name,'') AS host_name,
	             severity, category, summary_ko,
	             COALESCE(root_cause_ko,'') AS root_cause_ko,
	             COALESCE(action_ko,'')     AS action_ko,
	             needs_human, model_name,
	             COALESCE(prompt_tokens, 0),
	             COALESCE(output_tokens, 0),
	             COALESCE(latency_ms, 0),
	             created_at, COALESCE(incident_key,'')
	      FROM log_insights
	      WHERE 1=1`
	var args []any
	idx := 1
	if f.HostName != "" {
		q += fmt.Sprintf(" AND host_name = $%d", idx)
		args = append(args, f.HostName)
		idx++
	}
	if f.Severity != "" {
		q += fmt.Sprintf(" AND severity = $%d", idx)
		args = append(args, f.Severity)
		idx++
	}
	if !f.From.IsZero() {
		q += fmt.Sprintf(" AND created_at >= $%d", idx)
		args = append(args, f.From)
		idx++
	}
	if !f.To.IsZero() {
		q += fmt.Sprintf(" AND created_at <= $%d", idx)
		args = append(args, f.To)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
	args = append(args, f.Limit)
	return q, args
}

func (s *Store) getByID(ctx context.Context, id int64) (*Insight, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, template_hash, sample_log_ids, COALESCE(host_name,''),
		       severity, category, summary_ko,
		       COALESCE(root_cause_ko,''), COALESCE(action_ko,''),
		       needs_human, model_name,
		       COALESCE(prompt_tokens, 0), COALESCE(output_tokens, 0),
		       COALESCE(latency_ms, 0), created_at, COALESCE(incident_key,'')
		FROM log_insights WHERE id = $1`, id)
	ins, err := scanInsight(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ins, nil
}

// rowScanner — pgx.Row / pgx.Rows 둘 다 Scan을 가짐.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanInsight(r rowScanner) (Insight, error) {
	var ins Insight
	var severity, category string
	err := r.Scan(
		&ins.ID, &ins.TemplateHash, &ins.SampleLogIDs, &ins.HostName,
		&severity, &category, &ins.SummaryKo,
		&ins.RootCauseKo, &ins.ActionKo,
		&ins.NeedsHuman, &ins.ModelName,
		&ins.PromptTokens, &ins.OutputTokens, &ins.LatencyMs,
		&ins.CreatedAt, &ins.IncidentKey,
	)
	if err != nil {
		return Insight{}, err
	}
	ins.Severity = Severity(severity)
	ins.Category = Category(category)
	return ins, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
