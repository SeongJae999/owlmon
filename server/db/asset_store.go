package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Asset는 모니터링 대상 장비의 자산 정보입니다.
type Asset struct {
	ID              int64     `json:"id"`
	HostName        string    `json:"host_name"`
	IP              string    `json:"ip"`
	Location        string    `json:"location"`        // 위치 (예: 2층 서버실)
	Description     string    `json:"description"`     // 장비 설명
	PurchaseDate    string    `json:"purchase_date"`   // "YYYY-MM-DD" 또는 ""
	WarrantyExpires string    `json:"warranty_expires"` // "YYYY-MM-DD" 또는 ""
	Notes           string    `json:"notes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AssetStore struct {
	pool *pgxpool.Pool
}

func NewAssetStore(pool *pgxpool.Pool) *AssetStore {
	return &AssetStore{pool: pool}
}

// List는 모든 자산 정보를 반환합니다.
func (s *AssetStore) List(ctx context.Context) ([]Asset, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, host_name, ip, location, description,
		        COALESCE(purchase_date::text, ''),
		        COALESCE(warranty_expires::text, ''),
		        notes, updated_at
		 FROM assets ORDER BY host_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.HostName, &a.IP, &a.Location, &a.Description,
			&a.PurchaseDate, &a.WarrantyExpires, &a.Notes, &a.UpdatedAt); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

// Upsert는 자산을 생성하거나 host_name 기준으로 업데이트합니다.
func (s *AssetStore) Upsert(ctx context.Context, a Asset) (Asset, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO assets (host_name, ip, location, description, purchase_date, warranty_expires, notes, updated_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::date, NULLIF($6, '')::date, $7, NOW())
		 ON CONFLICT (host_name) DO UPDATE SET
		     ip               = EXCLUDED.ip,
		     location         = EXCLUDED.location,
		     description      = EXCLUDED.description,
		     purchase_date    = EXCLUDED.purchase_date,
		     warranty_expires = EXCLUDED.warranty_expires,
		     notes            = EXCLUDED.notes,
		     updated_at       = NOW()
		 RETURNING id, host_name, ip, location, description,
		           COALESCE(purchase_date::text, ''),
		           COALESCE(warranty_expires::text, ''),
		           notes, updated_at`,
		a.HostName, a.IP, a.Location, a.Description,
		a.PurchaseDate, a.WarrantyExpires, a.Notes,
	).Scan(&a.ID, &a.HostName, &a.IP, &a.Location, &a.Description,
		&a.PurchaseDate, &a.WarrantyExpires, &a.Notes, &a.UpdatedAt)
	return a, err
}

// Delete는 자산을 삭제합니다.
func (s *AssetStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM assets WHERE id=$1`, id)
	return err
}
