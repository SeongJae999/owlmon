package db

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultRetentionDays = 90
	partitionPrefix      = "logs_"
	// 파티션 이름 포맷: logs_YYYY_MM_DD (UTC 일자 기준)
	partitionDateLayout = "2006_01_02"
)

// LogPartitionManager는 logs 테이블의 일별 파티션을 자동 관리합니다.
//
// 동작:
//   - 시작 시: 오늘 + 내일 파티션을 미리 생성 (자정 경계 누락 방지)
//   - 매 1시간: 새 파티션 확보 + 보존기간 초과 파티션 DROP
//
// 모든 시각 계산은 UTC 기준입니다 (DST 무관).
type LogPartitionManager struct {
	pool          *pgxpool.Pool
	retentionDays int
}

// NewLogPartitionManager는 보존기간을 받아 매니저를 생성합니다.
// retentionDays <= 0이면 기본값 90을 사용합니다.
func NewLogPartitionManager(pool *pgxpool.Pool, retentionDays int) *LogPartitionManager {
	if retentionDays <= 0 {
		retentionDays = defaultRetentionDays
	}
	return &LogPartitionManager{pool: pool, retentionDays: retentionDays}
}

// Start는 백그라운드 루프를 시작합니다.
// 첫 호출 시 즉시 1회 실행하여 오늘/내일 파티션을 확보한 뒤 ticker 루프 진입.
//
// 호출자는 ctx를 cancel하여 루프를 정지시킬 수 있습니다.
func (m *LogPartitionManager) Start(ctx context.Context) {
	if err := m.ensureNearFuture(ctx); err != nil {
		log.Printf("[log-partition] 초기 파티션 생성 실패: %v", err)
	}
	go m.loop(ctx)
}

func (m *LogPartitionManager) loop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.ensureNearFuture(ctx); err != nil {
				log.Printf("[log-partition] 파티션 확보 실패: %v", err)
			}
			if err := m.dropExpired(ctx); err != nil {
				log.Printf("[log-partition] 만료 파티션 삭제 실패: %v", err)
			}
		}
	}
}

// ensureNearFuture는 오늘 + 내일 파티션을 멱등하게 생성합니다.
func (m *LogPartitionManager) ensureNearFuture(ctx context.Context) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for i := 0; i < 2; i++ {
		day := today.AddDate(0, 0, i)
		if err := m.createPartition(ctx, day); err != nil {
			return fmt.Errorf("create partition for %s: %w", day.Format(partitionDateLayout), err)
		}
	}
	return nil
}

// createPartition은 특정 날짜의 파티션을 생성합니다 (이미 있으면 no-op).
//
// 파티션 경계는 UTC 자정~다음 자정 (TIMESTAMPTZ 절대값).
func (m *LogPartitionManager) createPartition(ctx context.Context, day time.Time) error {
	day = day.UTC().Truncate(24 * time.Hour)
	name := partitionName(day)
	from := day.Format("2006-01-02 15:04:05+00")
	to := day.AddDate(0, 0, 1).Format("2006-01-02 15:04:05+00")

	// name과 경계값은 내부 생성이므로 SQL 인젝션 위험 없음
	sql := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF logs
		 FOR VALUES FROM ('%s') TO ('%s')`,
		name, from, to,
	)
	_, err := m.pool.Exec(ctx, sql)
	return err
}

// dropExpired는 retentionDays를 초과한 파티션을 DROP합니다.
//
// pg_inherits로 logs의 자식 파티션을 모두 조회한 뒤,
// 이름에서 날짜를 파싱하여 cutoff 이전 파티션을 삭제합니다.
// 이름 파싱 실패한 항목(수동 생성된 비표준 파티션 등)은 건너뜁니다.
func (m *LogPartitionManager) dropExpired(ctx context.Context) error {
	cutoff := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -m.retentionDays)

	rows, err := m.pool.Query(ctx,
		`SELECT inhrelid::regclass::text
		 FROM pg_inherits
		 WHERE inhparent = 'logs'::regclass`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var toDrop []string
	for rows.Next() {
		var qname string
		if err := rows.Scan(&qname); err != nil {
			return err
		}
		day, ok := parsePartitionDate(qname)
		if !ok {
			continue // 비표준 이름은 안전하게 무시
		}
		if day.Before(cutoff) {
			toDrop = append(toDrop, qname)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, qname := range toDrop {
		if _, err := m.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", qname)); err != nil {
			log.Printf("[log-partition] DROP %s 실패: %v", qname, err)
			continue
		}
		log.Printf("[log-partition] 만료 파티션 삭제: %s", qname)
	}
	return nil
}

// partitionName은 날짜로부터 파티션 테이블 이름을 만듭니다 (UTC 자정 기준).
func partitionName(day time.Time) string {
	return partitionPrefix + day.UTC().Format(partitionDateLayout)
}

// parsePartitionDate는 파티션 이름(또는 schema-qualified name)에서 날짜를 추출합니다.
// 형식이 맞지 않으면 ok=false 반환 (수동으로 만든 logs_archive 같은 비표준명 안전 처리).
func parsePartitionDate(qname string) (time.Time, bool) {
	// "public.logs_2026_04_24" → "logs_2026_04_24"
	if i := strings.LastIndex(qname, "."); i >= 0 {
		qname = qname[i+1:]
	}
	// 따옴표 제거 (regclass가 특수문자 포함 시 따옴표를 붙임)
	qname = strings.Trim(qname, `"`)

	if !strings.HasPrefix(qname, partitionPrefix) {
		return time.Time{}, false
	}
	day, err := time.Parse(partitionDateLayout, strings.TrimPrefix(qname, partitionPrefix))
	if err != nil {
		return time.Time{}, false
	}
	return day.UTC(), true
}
