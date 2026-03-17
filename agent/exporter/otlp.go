package exporter

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
)

// NewOTLPExporter는 OTel Collector로 메트릭을 전송하는 gRPC exporter를 생성합니다.
// endpoint 예시: "localhost:4317"
func NewOTLPExporter(ctx context.Context, endpoint string) (metric.Exporter, error) {
	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(), // MVP: TLS 없이 연결 (프로덕션에서는 제거)
		// Collector 일시 다운 시 재시도 설정
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: 5 * time.Second,  // 첫 재시도: 5초 후
			MaxInterval:     30 * time.Second, // 재시도 간격 최대 30초
			MaxElapsedTime:  5 * time.Minute,  // 5분간 재시도 후 포기
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("OTLP exporter 생성 실패: %w", err)
	}
	return exp, nil
}
