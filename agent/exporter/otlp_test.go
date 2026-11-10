package exporter

import (
	"context"
	"net"
	"slices"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestNewOTLPExporterAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name     string
		agentKey string
		want     []string
	}{
		{name: "키 있음", agentKey: "test-key", want: []string{"Bearer test-key"}},
		{name: "키 없음", agentKey: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("리스너 생성 실패: %v", err)
			}

			got := make(chan []string, 1)
			srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
				md, _ := metadata.FromIncomingContext(stream.Context())
				got <- md.Get("authorization")
				return status.Error(codes.Unauthenticated, "거부")
			}))
			go srv.Serve(lis)
			defer srv.Stop()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			exp, err := NewOTLPExporter(ctx, lis.Addr().String(), tt.agentKey)
			if err != nil {
				t.Fatalf("exporter 생성 실패: %v", err)
			}
			defer exp.Shutdown(context.Background())

			if err := exp.Export(ctx, &metricdata.ResourceMetrics{}); err == nil {
				t.Fatal("서버가 거부했는데 Export가 성공했다")
			}

			select {
			case header := <-got:
				if !slices.Equal(header, tt.want) {
					t.Errorf("authorization = %v, 기대값 %v", header, tt.want)
				}
			case <-ctx.Done():
				t.Fatal("서버가 요청을 받지 못했다")
			}
		})
	}
}
