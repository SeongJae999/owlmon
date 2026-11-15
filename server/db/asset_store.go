package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Asset는 모니터링 대상 장비의 자산 정보입니다.
type Asset struct {
	ID              int64     `json:"id"`
	HostName        string    `json:"host_name"`
	IP              string    `json:"ip"`
	Location        string    `json:"location"`         // 위치 (예: 2층 서버실)
	Description     string    `json:"description"`      // 장비 설명
	PurchaseDate    string    `json:"purchase_date"`    // "YYYY-MM-DD" 또는 ""
	WarrantyExpires string    `json:"warranty_expires"` // "YYYY-MM-DD" 또는 ""
	Notes           string    `json:"notes"`
	Rack            string    `json:"rack"`      // 빈 값이면 랙에 넣지 않은 장비
	RackUnit        int       `json:"rack_unit"` // 장비 맨 아래 칸 번호 (1 = 랙 맨 아래)
	RackSize        int       `json:"rack_size"` // 차지하는 칸 수
	UpdatedAt       time.Time `json:"updated_at"`
}

// RackError는 입력이 랙에 맞지 않을 때 — 화면에 그대로 보여 줄 문장
type RackError struct{ Msg string }

func (e *RackError) Error() string { return e.Msg }

type AssetStore struct {
	pool *pgxpool.Pool
}

func NewAssetStore(pool *pgxpool.Pool) *AssetStore {
	return &AssetStore{pool: pool}
}

// init.sql은 새 DB에서만 돌기 때문에 기존 운영 DB에도 생기도록 기동 시 추가
func (s *AssetStore) EnsureColumns(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS racks (
			name   TEXT PRIMARY KEY,
			room   TEXT NOT NULL DEFAULT '',
			height INT  NOT NULL DEFAULT 42 CHECK (height BETWEEN 1 AND 60)
		)`,
		`ALTER TABLE assets ADD COLUMN IF NOT EXISTS rack      TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE assets ADD COLUMN IF NOT EXISTS rack_unit INT  NOT NULL DEFAULT 0`,
		`ALTER TABLE assets ADD COLUMN IF NOT EXISTS rack_size INT  NOT NULL DEFAULT 1`,
	} {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

const assetCols = `id, host_name, ip, location, description,
	COALESCE(purchase_date::text, ''), COALESCE(warranty_expires::text, ''),
	notes, rack, rack_unit, rack_size, updated_at`

func scanAsset(row pgx.Row, a *Asset) error {
	return row.Scan(&a.ID, &a.HostName, &a.IP, &a.Location, &a.Description,
		&a.PurchaseDate, &a.WarrantyExpires, &a.Notes, &a.Rack, &a.RackUnit, &a.RackSize, &a.UpdatedAt)
}

// List는 모든 자산 정보를 반환합니다.
func (s *AssetStore) List(ctx context.Context) ([]Asset, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+assetCols+` FROM assets ORDER BY host_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []Asset
	for rows.Next() {
		var a Asset
		if err := scanAsset(rows, &a); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

// Upsert는 자산을 생성하거나 host_name 기준으로 업데이트합니다.
// 랙을 적었으면 높이·겹침을 같은 트랜잭션에서 확인 — 두 사람이 동시에 같은 칸에 넣지 못하게 랙 행을 잠근다
func (s *AssetStore) Upsert(ctx context.Context, a Asset) (Asset, error) {
	a.Rack = strings.TrimSpace(a.Rack)
	if a.Rack == "" {
		a.RackUnit, a.RackSize = 0, 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer tx.Rollback(ctx)

	if a.Rack != "" {
		// 처음 보는 랙 이름은 42U로 만든다 — 엑셀 일괄 등록에서 랙을 따로 만들지 않아도 되게
		if _, err := tx.Exec(ctx, `INSERT INTO racks (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, a.Rack); err != nil {
			return a, err
		}
		var height int
		err := tx.QueryRow(ctx, `SELECT height FROM racks WHERE name=$1 FOR UPDATE`, a.Rack).Scan(&height)
		if errors.Is(err, pgx.ErrNoRows) {
			return a, &RackError{a.Rack + "이(가) 방금 삭제되었습니다 — 다시 저장하세요"}
		}
		if err != nil {
			return a, err
		}
		if msg := rackFit(a.RackUnit, a.RackSize, height); msg != "" {
			return a, &RackError{a.Rack + ": " + msg}
		}
		var other string
		var ou, os int
		err = tx.QueryRow(ctx,
			`SELECT host_name, rack_unit, rack_size FROM assets
			 WHERE rack=$1 AND host_name<>$2 AND rack_unit <= $3 AND rack_unit + rack_size - 1 >= $4
			 ORDER BY rack_unit LIMIT 1`,
			a.Rack, a.HostName, a.RackUnit+a.RackSize-1, a.RackUnit).Scan(&other, &ou, &os)
		if err == nil {
			return a, &RackError{fmt.Sprintf("%s %s에 이미 %s가 있습니다", a.Rack, UnitRange(ou, os), other)}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return a, err
		}
	}

	err = scanAsset(tx.QueryRow(ctx,
		`INSERT INTO assets (host_name, ip, location, description, purchase_date, warranty_expires, notes, rack, rack_unit, rack_size, updated_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::date, NULLIF($6, '')::date, $7, $8, $9, $10, NOW())
		 ON CONFLICT (host_name) DO UPDATE SET
		     ip               = EXCLUDED.ip,
		     location         = EXCLUDED.location,
		     description      = EXCLUDED.description,
		     purchase_date    = EXCLUDED.purchase_date,
		     warranty_expires = EXCLUDED.warranty_expires,
		     notes            = EXCLUDED.notes,
		     rack             = EXCLUDED.rack,
		     rack_unit        = EXCLUDED.rack_unit,
		     rack_size        = EXCLUDED.rack_size,
		     updated_at       = NOW()
		 RETURNING `+assetCols,
		a.HostName, a.IP, a.Location, a.Description,
		a.PurchaseDate, a.WarrantyExpires, a.Notes, a.Rack, a.RackUnit, a.RackSize,
	), &a)
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}

func rackFit(unit, size, height int) string {
	if unit < 1 || size < 1 {
		return "시작 U와 U 수는 1 이상이어야 합니다"
	}
	if unit+size-1 > height {
		return fmt.Sprintf("랙 높이(%dU)를 넘습니다 — %s", height, UnitRange(unit, size))
	}
	return ""
}

func UnitRange(unit, size int) string {
	if size <= 1 {
		return fmt.Sprintf("%dU", unit)
	}
	return fmt.Sprintf("%d~%dU", unit, unit+size-1)
}

// 알림 메일·화면에 붙일 위치 — "본관 2층 서버실 · A랙 14U"
func LocationText(location, rack string, unit, size int) string {
	parts := []string{}
	if l := strings.TrimSpace(location); l != "" {
		parts = append(parts, l)
	}
	if rack != "" && unit > 0 {
		parts = append(parts, rack+" "+UnitRange(unit, size))
	}
	return strings.Join(parts, " · ")
}

// 없는 장비는 빈 문자열 — 알림 메일에 위치 줄을 붙일지 판단용
func (s *AssetStore) LocationOf(ctx context.Context, host string) (string, error) {
	var loc, rack string
	var unit, size int
	err := s.pool.QueryRow(ctx, `SELECT location, rack, rack_unit, rack_size FROM assets WHERE host_name=$1`, host).Scan(&loc, &rack, &unit, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return LocationText(loc, rack, unit, size), err
}

// Delete는 자산을 삭제합니다.
func (s *AssetStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM assets WHERE id=$1`, id)
	return err
}
