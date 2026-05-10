package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentSpecs는 한 호스트의 하드웨어/OS 스냅샷이다.
// 에이전트가 시작 시 1회 보내며, 같은 호스트가 다시 보내면 UPSERT로 갱신된다.
type AgentSpecs struct {
	HostName         string          `json:"host_name"`
	CPUModel         string          `json:"cpu_model"`
	CPUCores         int             `json:"cpu_cores"`
	CPUSockets       int             `json:"cpu_sockets"`
	MemoryTotalBytes int64           `json:"memory_total_bytes"`
	Disks            json.RawMessage `json:"disks"`     // [{name,size_bytes,rotational,model}, ...]
	Networks         json.RawMessage `json:"networks"`  // [{name,mac,ipv4}, ...]
	OSPrettyName     string          `json:"os_pretty_name"`
	KernelVersion    string          `json:"kernel_version"`
	Virtualization   string          `json:"virtualization"`
	Arch             string          `json:"arch"`
	CollectedAt      time.Time       `json:"collected_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type SpecsStore struct {
	pool *pgxpool.Pool
}

func NewSpecsStore(pool *pgxpool.Pool) *SpecsStore {
	return &SpecsStore{pool: pool}
}

// Upsert는 host_name 기준으로 스펙을 INSERT 또는 UPDATE한다.
// 에이전트가 재시작될 때마다 호출되므로 멱등성 보장.
func (s *SpecsStore) Upsert(ctx context.Context, spec *AgentSpecs) error {
	const q = `
INSERT INTO agent_specs
  (host_name, cpu_model, cpu_cores, cpu_sockets, memory_total_bytes,
   disks, networks, os_pretty_name, kernel_version, virtualization, arch,
   collected_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, NOW(), NOW())
ON CONFLICT (host_name) DO UPDATE SET
  cpu_model          = EXCLUDED.cpu_model,
  cpu_cores          = EXCLUDED.cpu_cores,
  cpu_sockets        = EXCLUDED.cpu_sockets,
  memory_total_bytes = EXCLUDED.memory_total_bytes,
  disks              = EXCLUDED.disks,
  networks           = EXCLUDED.networks,
  os_pretty_name     = EXCLUDED.os_pretty_name,
  kernel_version     = EXCLUDED.kernel_version,
  virtualization     = EXCLUDED.virtualization,
  arch               = EXCLUDED.arch,
  updated_at         = NOW()
`
	_, err := s.pool.Exec(ctx, q,
		spec.HostName, spec.CPUModel, spec.CPUCores, spec.CPUSockets, spec.MemoryTotalBytes,
		spec.Disks, spec.Networks, spec.OSPrettyName, spec.KernelVersion, spec.Virtualization, spec.Arch,
	)
	return err
}

// List는 모든 호스트 스펙을 반환 (host_name ASC).
func (s *SpecsStore) List(ctx context.Context) ([]AgentSpecs, error) {
	const q = `
SELECT host_name, cpu_model, cpu_cores, cpu_sockets, memory_total_bytes,
       disks, networks, os_pretty_name, kernel_version, virtualization, arch,
       collected_at, updated_at
FROM agent_specs
ORDER BY host_name ASC
`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AgentSpecs
	for rows.Next() {
		var sp AgentSpecs
		if err := rows.Scan(
			&sp.HostName, &sp.CPUModel, &sp.CPUCores, &sp.CPUSockets, &sp.MemoryTotalBytes,
			&sp.Disks, &sp.Networks, &sp.OSPrettyName, &sp.KernelVersion, &sp.Virtualization, &sp.Arch,
			&sp.CollectedAt, &sp.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, sp)
	}
	return result, rows.Err()
}

// Get은 단일 호스트의 스펙을 반환. 없으면 nil, nil.
func (s *SpecsStore) Get(ctx context.Context, hostName string) (*AgentSpecs, error) {
	const q = `
SELECT host_name, cpu_model, cpu_cores, cpu_sockets, memory_total_bytes,
       disks, networks, os_pretty_name, kernel_version, virtualization, arch,
       collected_at, updated_at
FROM agent_specs WHERE host_name = $1
`
	var sp AgentSpecs
	err := s.pool.QueryRow(ctx, q, hostName).Scan(
		&sp.HostName, &sp.CPUModel, &sp.CPUCores, &sp.CPUSockets, &sp.MemoryTotalBytes,
		&sp.Disks, &sp.Networks, &sp.OSPrettyName, &sp.KernelVersion, &sp.Virtualization, &sp.Arch,
		&sp.CollectedAt, &sp.UpdatedAt,
	)
	if err != nil {
		// pgx.ErrNoRows 처리는 호출 측이 errors.Is로 — 여기선 그대로 반환
		return nil, err
	}
	return &sp, nil
}
