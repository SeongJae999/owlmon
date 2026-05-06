package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LogAnnotation은 운영자가 로그에 부여한 원인/조치 라벨입니다.
//
// logs 테이블이 retention 정책으로 삭제돼도 이 라벨은 영구 보관됩니다 —
// 미래 LLM 파인튜닝 시 (로그 → 원인/조치) 페어 학습 데이터로 활용 예정.
type LogAnnotation struct {
	ID           int64     `json:"id"`
	LogID        int64     `json:"log_id"`
	LogTimestamp time.Time `json:"log_timestamp"`
	Annotator    string    `json:"annotator"`
	Category     string    `json:"category,omitempty"` // 'root_cause' | 'action_taken' | 'false_positive'
	Problem      string    `json:"problem,omitempty"`
	Solution     string    `json:"solution,omitempty"`
	AlertID      *int64    `json:"alert_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// LogAnnotationStore는 log_annotations 테이블 저장소입니다.
type LogAnnotationStore struct {
	pool *pgxpool.Pool
}

func NewLogAnnotationStore(pool *pgxpool.Pool) *LogAnnotationStore {
	return &LogAnnotationStore{pool: pool}
}

// Insert는 새 라벨을 저장합니다. 같은 log_id에 여러 라벨 가능.
// 반환 값에 생성된 id와 created_at이 채워집니다.
func (s *LogAnnotationStore) Insert(ctx context.Context, a *LogAnnotation) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO log_annotations
		 (log_id, log_timestamp, annotator, category, problem, solution, alert_id)
		 VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7)
		 RETURNING id, created_at`,
		a.LogID, a.LogTimestamp, a.Annotator, a.Category, a.Problem, a.Solution, a.AlertID,
	).Scan(&a.ID, &a.CreatedAt)
}

// ListByLogID는 특정 로그에 부여된 모든 라벨을 시간 역순으로 반환합니다.
func (s *LogAnnotationStore) ListByLogID(ctx context.Context, logID int64) ([]LogAnnotation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, log_id, log_timestamp, annotator,
		        COALESCE(category, ''), COALESCE(problem, ''), COALESCE(solution, ''),
		        alert_id, created_at
		 FROM log_annotations
		 WHERE log_id = $1
		 ORDER BY created_at DESC`,
		logID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []LogAnnotation
	for rows.Next() {
		var a LogAnnotation
		if err := rows.Scan(&a.ID, &a.LogID, &a.LogTimestamp, &a.Annotator,
			&a.Category, &a.Problem, &a.Solution, &a.AlertID, &a.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// List는 최근 라벨 목록을 페이지네이션해서 반환합니다 (학습 데이터 추출용).
func (s *LogAnnotationStore) List(ctx context.Context, limit, offset int) ([]LogAnnotation, int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM log_annotations`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, log_id, log_timestamp, annotator,
		        COALESCE(category, ''), COALESCE(problem, ''), COALESCE(solution, ''),
		        alert_id, created_at
		 FROM log_annotations
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []LogAnnotation
	for rows.Next() {
		var a LogAnnotation
		if err := rows.Scan(&a.ID, &a.LogID, &a.LogTimestamp, &a.Annotator,
			&a.Category, &a.Problem, &a.Solution, &a.AlertID, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, a)
	}
	return result, total, rows.Err()
}

// Delete는 라벨을 삭제합니다 (오타 수정 등 운영 편의).
func (s *LogAnnotationStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM log_annotations WHERE id = $1`, id)
	return err
}
