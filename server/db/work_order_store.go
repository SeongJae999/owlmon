package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 작업 상태 — 할 일 / 하는 중 / 끝남
var WorkOrderStatuses = map[string]bool{"todo": true, "doing": true, "done": true}

// ErrWorkOrderNotFound는 없는 작업을 고치거나 지우려 할 때
var ErrWorkOrderNotFound = errors.New("작업이 없습니다")

// WorkOrder는 장비 하나에 남기는 "언제까지 무엇을" 메모. 기한은 날짜만 (시각은 현장에서 의미가 없음)
type WorkOrder struct {
	ID          int64      `json:"id"`
	HostName    string     `json:"host_name"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *string    `json:"due_date"` // "YYYY-MM-DD" 또는 null(기한 없음)
	Assignee    string     `json:"assignee"`
	Status      string     `json:"status"`
	AlertRef    *string    `json:"alert_ref"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DoneNote    string     `json:"done_note"`
	DoneAt      *time.Time `json:"done_at"`
}

// WorkOrderFilter의 빈 값은 조건 없음
type WorkOrderFilter struct {
	Status string
	Host   string
}

type WorkOrderStore struct {
	pool *pgxpool.Pool
}

func NewWorkOrderStore(pool *pgxpool.Pool) *WorkOrderStore {
	return &WorkOrderStore{pool: pool}
}

// init.sql은 새 DB에서만 돌기 때문에 기존 운영 DB에도 생기도록 기동 시 생성
func (s *WorkOrderStore) EnsureTable(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS work_orders (
			id          BIGSERIAL PRIMARY KEY,
			host_name   TEXT NOT NULL CHECK (host_name <> ''),
			title       TEXT NOT NULL CHECK (title <> ''),
			description TEXT NOT NULL DEFAULT '',
			due_date    DATE,
			assignee    TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'doing', 'done')),
			alert_ref   TEXT,
			created_by  TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			done_note   TEXT NOT NULL DEFAULT '',
			done_at     TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS work_orders_open ON work_orders (status, due_date);
		CREATE INDEX IF NOT EXISTS work_orders_host ON work_orders (host_name)`)
	return err
}

const workOrderCols = `id, host_name, title, description, to_char(due_date, 'YYYY-MM-DD'), assignee, status, alert_ref,
	created_by, created_at, updated_at, done_note, done_at`

func scanWorkOrder(row pgx.Row) (WorkOrder, error) {
	var w WorkOrder
	err := row.Scan(&w.ID, &w.HostName, &w.Title, &w.Description, &w.DueDate, &w.Assignee, &w.Status, &w.AlertRef,
		&w.CreatedBy, &w.CreatedAt, &w.UpdatedAt, &w.DoneNote, &w.DoneAt)
	return w, err
}

// List는 기한이 급한 순 — 기한 없는 작업은 맨 뒤, 같은 기한이면 먼저 등록한 것부터
func (s *WorkOrderStore) List(ctx context.Context, f WorkOrderFilter) ([]WorkOrder, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+workOrderCols+` FROM work_orders
		WHERE ($1 = '' OR status = $1) AND ($2 = '' OR host_name = $2)
		ORDER BY due_date ASC NULLS LAST, created_at ASC, id ASC`, f.Status, f.Host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkOrder{}
	for rows.Next() {
		w, err := scanWorkOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Create는 새 작업을 넣고 저장된 행을 그대로 돌려줌. 상태는 늘 '할 일'로 시작
func (s *WorkOrderStore) Create(ctx context.Context, w WorkOrder) (WorkOrder, error) {
	return scanWorkOrder(s.pool.QueryRow(ctx, `INSERT INTO work_orders (host_name, title, description, due_date, assignee, alert_ref, created_by)
		VALUES ($1, $2, $3, $4::text::date, $5, $6, $7) RETURNING `+workOrderCols,
		w.HostName, w.Title, w.Description, w.DueDate, w.Assignee, w.AlertRef, w.CreatedBy))
}

// Update는 내용(장비·제목·설명·기한·담당자·알림 참조)만 바꿈 — 상태는 SetStatus로만 (조회 계정도 쓰는 길이라 따로 둠)
func (s *WorkOrderStore) Update(ctx context.Context, w WorkOrder) (WorkOrder, error) {
	out, err := scanWorkOrder(s.pool.QueryRow(ctx, `UPDATE work_orders
		SET host_name = $2, title = $3, description = $4, due_date = $5::text::date, assignee = $6, alert_ref = $7, updated_at = now()
		WHERE id = $1 RETURNING `+workOrderCols,
		w.ID, w.HostName, w.Title, w.Description, w.DueDate, w.Assignee, w.AlertRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrWorkOrderNotFound
	}
	return out, err
}

// SetStatus는 상태와 완료 메모를 바꿈. 끝남으로 바꿀 때만 완료 시각을 찍고(이미 끝난 작업은 처음 시각 유지), 다시 열면 지움
func (s *WorkOrderStore) SetStatus(ctx context.Context, id int64, status, note string) (WorkOrder, error) {
	out, err := scanWorkOrder(s.pool.QueryRow(ctx, `UPDATE work_orders
		SET status = $2, done_note = $3, updated_at = now(),
			done_at = CASE WHEN $2 = 'done' THEN COALESCE(done_at, now()) ELSE NULL END
		WHERE id = $1 RETURNING `+workOrderCols, id, status, note))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrWorkOrderNotFound
	}
	return out, err
}

func (s *WorkOrderStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM work_orders WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrWorkOrderNotFound
	}
	return nil
}
