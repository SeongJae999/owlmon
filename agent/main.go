package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/seongJae/owlmon/agent/collector"
	"github.com/seongJae/owlmon/agent/config"
	"github.com/seongJae/owlmon/agent/exporter"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func main() {
	// 설정 파일 로드 (없으면 기본값으로 동작)
	cfg, err := config.Load(getEnv("OWLMON_CONFIG", "config.yaml"))
	if err != nil {
		log.Printf("설정 파일 로드 실패, 기본값 사용: %v", err)
		cfg = &config.Config{}
	}

	// 환경변수가 설정 파일보다 우선
	endpoint := getEnv("OWLMON_OTLP_ENDPOINT", cfg.OTLPEndpoint)
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// OTLP gRPC exporter 생성 (버퍼링 래퍼 포함)
	otlpExp, err := exporter.NewOTLPExporter(ctx, endpoint)
	if err != nil {
		log.Fatalf("exporter 초기화 실패: %v", err)
	}
	exp := exporter.NewBufferedExporter(otlpExp)

	// 리소스 정보 (어느 호스트에서 보낸 메트릭인지 식별)
	hostname, _ := os.Hostname()
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("owlmon-agent"),
		semconv.ServiceVersion("0.1.0"),
		semconv.HostName(hostname),
	)

	// MeterProvider 생성 (30초마다 수집 후 전송)
	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exp,
			metric.WithInterval(30*time.Second),
		)),
		metric.WithResource(res),
	)
	defer func() {
		if err := provider.Shutdown(ctx); err != nil {
			log.Printf("MeterProvider 종료 실패: %v", err)
		}
	}()

	meter := provider.Meter("owlmon.agent")

	// 시스템 메트릭 수집기 초기화
	if _, err := collector.NewCPUCollector(meter); err != nil {
		log.Fatalf("CPU 수집기 초기화 실패: %v", err)
	}
	if _, err := collector.NewMemoryCollector(meter); err != nil {
		log.Fatalf("메모리 수집기 초기화 실패: %v", err)
	}
	if _, err := collector.NewDiskCollector(meter); err != nil {
		log.Fatalf("디스크 수집기 초기화 실패: %v", err)
	}

	// 서비스 체크 수집기 초기화 (설정 파일에 checks가 있을 때만)
	if _, err := collector.NewServiceCheckCollector(meter, cfg.Checks); err != nil {
		log.Fatalf("서비스 체크 수집기 초기화 실패: %v", err)
	}

	log.Printf("owlmon-agent 시작 (호스트: %s, endpoint: %s)", hostname, endpoint)
	log.Printf("수집 주기: 30초 | 서비스 체크: %d개", len(cfg.Checks))

	// 종료 시그널 대기 (Ctrl+C, SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("owlmon-agent 종료 중...")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
