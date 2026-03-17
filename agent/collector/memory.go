package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/mem"
	"go.opentelemetry.io/otel/metric"
)

// MemoryCollector는 메모리 사용률과 사용량을 수집합니다.
type MemoryCollector struct {
	usageGauge metric.Float64ObservableGauge
	usedGauge  metric.Int64ObservableGauge
}

// NewMemoryCollector는 MemoryCollector를 생성하고 OTel 미터에 게이지를 등록합니다.
func NewMemoryCollector(meter metric.Meter) (*MemoryCollector, error) {
	usageGauge, err := meter.Float64ObservableGauge(
		"system.memory.usage_percent",
		metric.WithDescription("메모리 사용률 (%)"),
		metric.WithUnit("%"),
	)
	if err != nil {
		return nil, fmt.Errorf("메모리 사용률 게이지 생성 실패: %w", err)
	}

	usedGauge, err := meter.Int64ObservableGauge(
		"system.memory.used_bytes",
		metric.WithDescription("메모리 사용량 (bytes)"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("메모리 사용량 게이지 생성 실패: %w", err)
	}

	c := &MemoryCollector{
		usageGauge: usageGauge,
		usedGauge:  usedGauge,
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stat, err := mem.VirtualMemory()
		if err != nil {
			return fmt.Errorf("메모리 정보 수집 실패: %w", err)
		}
		o.ObserveFloat64(usageGauge, stat.UsedPercent)
		o.ObserveInt64(usedGauge, int64(stat.Used))
		return nil
	}, usageGauge, usedGauge)
	if err != nil {
		return nil, fmt.Errorf("메모리 콜백 등록 실패: %w", err)
	}

	return c, nil
}
