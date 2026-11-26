package exporter

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc/credentials"
)

// NewOTLPExporter는 OTel Collector로 메트릭을 전송하는 gRPC exporter를 생성합니다.
// endpoint 예시: "owlmon.school.local:443" (TLS, nginx) / "localhost:14317" (개발, 평문)
// tlsCfg가 nil이면 평문 — 예전 설치(4317)와 개발 환경용.
func NewOTLPExporter(ctx context.Context, endpoint, agentKey string, tlsCfg *tls.Config) (metric.Exporter, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
		// Collector 일시 다운 시 재시도 설정
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: 5 * time.Second,  // 첫 재시도: 5초 후
			MaxInterval:     30 * time.Second, // 재시도 간격 최대 30초
			MaxElapsedTime:  5 * time.Minute,  // 5분간 재시도 후 포기
		}),
	}
	if tlsCfg != nil {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	if agentKey != "" {
		// nginx가 authorization 헤더의 키를 서버에 확인받는다
		opts = append(opts, otlpmetricgrpc.WithHeaders(map[string]string{"authorization": "Bearer " + agentKey}))
	}

	exp, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("OTLP exporter 생성 실패: %w", err)
	}
	return exp, nil
}
