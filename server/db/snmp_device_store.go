package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/snmp"
)

// SNMPDeviceStore는 PostgreSQL 기반 SNMP 장비 저장소입니다.
type SNMPDeviceStore struct {
	pool *pgxpool.Pool
}

func NewSNMPDeviceStore(pool *pgxpool.Pool) *SNMPDeviceStore {
	return &SNMPDeviceStore{pool: pool}
}

func (s *SNMPDeviceStore) List(ctx context.Context) ([]snmp.Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, ip, community, port FROM snmp_devices ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []snmp.Device
	for rows.Next() {
		var d snmp.Device
		if err := rows.Scan(&d.ID, &d.Name, &d.IP, &d.Community, &d.Port); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, nil
}

func (s *SNMPDeviceStore) Add(ctx context.Context, d snmp.Device) (snmp.Device, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO snmp_devices (name, ip, community, port)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		d.Name, d.IP, d.Community, d.Port,
	).Scan(&d.ID)
	return d, err
}

func (s *SNMPDeviceStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM snmp_devices WHERE id=$1`, id)
	return err
}
