package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/disk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DiskCollector는 디스크 사용률 + 절대값(GB)을 수집합니다.
type DiskCollector struct {
	usageGauge metric.Float64ObservableGauge
	usedGauge  metric.Int64ObservableGauge
	totalGauge metric.Int64ObservableGauge
}

// NewDiskCollector는 DiskCollector를 생성하고 OTel 미터에 게이지를 등록합니다.
func NewDiskCollector(meter metric.Meter) (*DiskCollector, error) {
	usageGauge, err := meter.Float64ObservableGauge(
		"system.disk.usage_percent",
		metric.WithDescription("디스크 사용률 (%)"),
		metric.WithUnit("%"),
	)
	if err != nil {
		return nil, fmt.Errorf("디스크 게이지 생성 실패: %w", err)
	}
	usedGauge, err := meter.Int64ObservableGauge(
		"system.disk.used_bytes",
		metric.WithDescription("디스크 사용 용량 (bytes)"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("디스크 used 게이지 생성 실패: %w", err)
	}
	totalGauge, err := meter.Int64ObservableGauge(
		"system.disk.total_bytes",
		metric.WithDescription("디스크 전체 용량 (bytes)"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("디스크 total 게이지 생성 실패: %w", err)
	}

	c := &DiskCollector{usageGauge: usageGauge, usedGauge: usedGauge, totalGauge: totalGauge}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		partitions, err := disk.Partitions(false) // 물리 디스크만
		if err != nil {
			return fmt.Errorf("파티션 목록 수집 실패: %w", err)
		}

		for _, p := range partitions {
			// 가상/pseudo 파일시스템 제외 (devfs, tmpfs, autofs 등)
			switch p.Fstype {
			case "devfs", "tmpfs", "autofs", "overlay", "squashfs", "fuse", "fusectl",
				"cgroup", "cgroup2", "proc", "sysfs", "debugfs", "securityfs":
				continue
			}

			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue // 접근 불가 파티션은 건너뜀
			}
			if usage.Total == 0 {
				continue // 전체 용량 0 (autofs 등 가상 마운트) 제외
			}
			attrs := metric.WithAttributeSet(attribute.NewSet(
				attribute.String("mountpoint", p.Mountpoint),
				attribute.String("device", p.Device),
			))
			o.ObserveFloat64(usageGauge, usage.UsedPercent, attrs)
			o.ObserveInt64(usedGauge, int64(usage.Used), attrs)
			o.ObserveInt64(totalGauge, int64(usage.Total), attrs)
		}
		return nil
	}, usageGauge, usedGauge, totalGauge)
	if err != nil {
		return nil, fmt.Errorf("디스크 콜백 등록 실패: %w", err)
	}

	return c, nil
}
