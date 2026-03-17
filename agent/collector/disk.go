package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/disk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DiskCollector는 디스크 사용률을 수집합니다.
type DiskCollector struct {
	usageGauge metric.Float64ObservableGauge
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

	c := &DiskCollector{usageGauge: usageGauge}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		partitions, err := disk.Partitions(false) // 물리 디스크만
		if err != nil {
			return fmt.Errorf("파티션 목록 수집 실패: %w", err)
		}

		for _, p := range partitions {
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue // 접근 불가 파티션은 건너뜀
			}
			o.ObserveFloat64(usageGauge, usage.UsedPercent,
				metric.WithAttributeSet(attribute.NewSet(
					attribute.String("mountpoint", p.Mountpoint),
					attribute.String("device", p.Device),
				)),
			)
		}
		return nil
	}, usageGauge)
	if err != nil {
		return nil, fmt.Errorf("디스크 콜백 등록 실패: %w", err)
	}

	return c, nil
}
