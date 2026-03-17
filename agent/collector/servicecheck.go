package collector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/seongJae/owlmon/agent/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ServiceCheckCollector는 HTTP/TCP 서비스 체크를 수행합니다.
type ServiceCheckCollector struct {
	checks []config.CheckConfig
}

// NewServiceCheckCollector는 설정에 따라 서비스 체크 수집기를 생성하고
// OTel 미터에 게이지를 등록합니다.
func NewServiceCheckCollector(meter metric.Meter, checks []config.CheckConfig) (*ServiceCheckCollector, error) {
	if len(checks) == 0 {
		return &ServiceCheckCollector{}, nil
	}

	// 서비스 상태 게이지 (1 = 정상, 0 = 장애)
	statusGauge, err := meter.Int64ObservableGauge(
		"service.check.status",
		metric.WithDescription("서비스 상태 (1=정상, 0=장애)"),
	)
	if err != nil {
		return nil, fmt.Errorf("서비스 상태 게이지 생성 실패: %w", err)
	}

	// 응답시간 게이지 (unit 생략 - OTel이 자동으로 suffix 추가하는 것 방지)
	latencyGauge, err := meter.Float64ObservableGauge(
		"service.check.latency_ms",
		metric.WithDescription("서비스 응답시간 (ms)"),
	)
	if err != nil {
		return nil, fmt.Errorf("응답시간 게이지 생성 실패: %w", err)
	}

	c := &ServiceCheckCollector{checks: checks}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, check := range c.checks {
			attrs := metric.WithAttributeSet(attribute.NewSet(
				attribute.String("check_name", check.Name),
				attribute.String("check_type", check.Type),
				attribute.String("target", check.Target),
			))

			var status int64
			var latencyMs float64

			switch check.Type {
			case "http":
				status, latencyMs = checkHTTP(check.Target)
			case "tcp":
				status, latencyMs = checkTCP(check.Target)
			}

			o.ObserveInt64(statusGauge, status, attrs)
			o.ObserveFloat64(latencyGauge, latencyMs, attrs)
		}
		return nil
	}, statusGauge, latencyGauge)
	if err != nil {
		return nil, fmt.Errorf("서비스 체크 콜백 등록 실패: %w", err)
	}

	return c, nil
}

// checkHTTP는 HTTP GET 요청을 보내고 상태(1/0)와 응답시간(ms)을 반환합니다.
func checkHTTP(url string) (status int64, latencyMs float64) {
	client := &http.Client{Timeout: 10 * time.Second}

	start := time.Now()
	resp, err := client.Get(url)
	latencyMs = float64(time.Since(start).Milliseconds())

	if err != nil || resp.StatusCode >= 500 {
		return 0, latencyMs
	}
	resp.Body.Close()
	return 1, latencyMs
}

// checkTCP는 TCP 연결을 시도하고 상태(1/0)와 응답시간(ms)을 반환합니다.
func checkTCP(address string) (status int64, latencyMs float64) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	latencyMs = float64(time.Since(start).Milliseconds())

	if err != nil {
		return 0, latencyMs
	}
	conn.Close()
	return 1, latencyMs
}
