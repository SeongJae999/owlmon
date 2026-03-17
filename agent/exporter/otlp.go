package exporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
)

// NewOTLPExporter는 OTel Collector로 메트릭을 전송하는 gRPC exporter를 생성합니다.
// endpoint 예시: "localhost:4317"
func NewOTLPExporter(ctx context.Context, endpoint string) (metric.Exporter, error) {
	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(), // MVP: TLS 없이 연결 (프로덕션에서는 제거)
	)
	if err != nil {
		return nil, fmt.Errorf("OTLP exporter 생성 실패: %w", err)
	}
	return exp, nil
}
