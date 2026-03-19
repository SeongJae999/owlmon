package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/net"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// NetworkCollector는 네트워크 인터페이스별 송수신 트래픽(bytes/s)을 수집합니다.
type NetworkCollector struct{}

func NewNetworkCollector(meter metric.Meter) (*NetworkCollector, error) {
	rxGauge, err := meter.Float64ObservableGauge(
		"system.network.rx_bytes_per_second",
		metric.WithDescription("네트워크 수신 속도 (bytes/s)"),
	)
	if err != nil {
		return nil, fmt.Errorf("network rx 게이지 생성 실패: %w", err)
	}

	txGauge, err := meter.Float64ObservableGauge(
		"system.network.tx_bytes_per_second",
		metric.WithDescription("네트워크 송신 속도 (bytes/s)"),
	)
	if err != nil {
		return nil, fmt.Errorf("network tx 게이지 생성 실패: %w", err)
	}

	// 이전 샘플을 저장해서 delta 계산
	type sample struct {
		rx, tx  uint64
		at      time.Time
	}
	prev := map[string]sample{}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stats, err := net.IOCounters(true) // 인터페이스별
		if err != nil {
			return err
		}
		now := time.Now()
		for _, s := range stats {
			// 루프백 제외
			if s.Name == "lo" || s.Name == "Loopback Pseudo-Interface 1" {
				continue
			}
			attrs := metric.WithAttributeSet(attribute.NewSet(
				attribute.String("interface", s.Name),
			))
			if p, ok := prev[s.Name]; ok {
				elapsed := now.Sub(p.at).Seconds()
				if elapsed > 0 {
					rxRate := float64(s.BytesRecv-p.rx) / elapsed
					txRate := float64(s.BytesSent-p.tx) / elapsed
					if rxRate >= 0 {
						o.ObserveFloat64(rxGauge, rxRate, attrs)
					}
					if txRate >= 0 {
						o.ObserveFloat64(txGauge, txRate, attrs)
					}
				}
			}
			prev[s.Name] = sample{rx: s.BytesRecv, tx: s.BytesSent, at: now}
		}
		return nil
	}, rxGauge, txGauge)
	if err != nil {
		return nil, fmt.Errorf("network 콜백 등록 실패: %w", err)
	}

	return &NetworkCollector{}, nil
}
