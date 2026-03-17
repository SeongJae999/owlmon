package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"go.opentelemetry.io/otel/metric"
)

// CPUCollector는 CPU 사용률을 수집합니다.
type CPUCollector struct {
	gauge metric.Float64ObservableGauge
}

// NewCPUCollector는 CPUCollector를 생성하고 OTel 미터에 게이지를 등록합니다.
func NewCPUCollector(meter metric.Meter) (*CPUCollector, error) {
	gauge, err := meter.Float64ObservableGauge(
		"system.cpu.usage",
		metric.WithDescription("CPU 사용률 (%)"),
		metric.WithUnit("%"),
	)
	if err != nil {
		return nil, fmt.Errorf("CPU 게이지 생성 실패: %w", err)
	}

	c := &CPUCollector{gauge: gauge}

	// 수집 콜백 등록
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		percents, err := cpu.Percent(0, false) // 전체 CPU 평균
		if err != nil {
			return fmt.Errorf("CPU 사용률 수집 실패: %w", err)
		}
		if len(percents) > 0 {
			o.ObserveFloat64(gauge, percents[0])
		}
		return nil
	}, gauge)
	if err != nil {
		return nil, fmt.Errorf("CPU 콜백 등록 실패: %w", err)
	}

	return c, nil
}
