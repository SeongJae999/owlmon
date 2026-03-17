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
	"github.com/seongJae/owlmon/agent/service"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func main() {
	if service.IsService() {
		// Windows 서비스 모드
		if err := service.Run(startAgent); err != nil {
			log.Fatalf("서비스 실행 실패: %v", err)
		}
		return
	}

	// 일반 콘솔 모드
	stop := startAgent()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("owlmon-agent 종료 중...")
	stop()
}

// startAgent는 에이전트를 시작하고 정지 함수를 반환합니다.
func startAgent() func() {
	// 설정 파일 로드
	cfg, err := config.Load(getEnv("OWLMON_CONFIG", "config.yaml"))
	if err != nil {
		log.Printf("설정 파일 로드 실패, 기본값 사용: %v", err)
		cfg = &config.Config{}
	}

	endpoint := getEnv("OWLMON_OTLP_ENDPOINT", cfg.OTLPEndpoint)
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	ctx, cancel := context.WithCancel(context.Background())

	otlpExp, err := exporter.NewOTLPExporter(ctx, endpoint)
	if err != nil {
		log.Fatalf("exporter 초기화 실패: %v", err)
	}
	exp := exporter.NewBufferedExporter(otlpExp)

	hostname, _ := os.Hostname()
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("owlmon-agent"),
		semconv.ServiceVersion("0.1.0"),
		semconv.HostName(hostname),
	)

	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exp,
			metric.WithInterval(30*time.Second),
		)),
		metric.WithResource(res),
	)

	meter := provider.Meter("owlmon.agent")

	if _, err := collector.NewCPUCollector(meter); err != nil {
		log.Fatalf("CPU 수집기 초기화 실패: %v", err)
	}
	if _, err := collector.NewMemoryCollector(meter); err != nil {
		log.Fatalf("메모리 수집기 초기화 실패: %v", err)
	}
	if _, err := collector.NewDiskCollector(meter); err != nil {
		log.Fatalf("디스크 수집기 초기화 실패: %v", err)
	}
	if _, err := collector.NewServiceCheckCollector(meter, cfg.Checks); err != nil {
		log.Fatalf("서비스 체크 수집기 초기화 실패: %v", err)
	}

	log.Printf("owlmon-agent 시작 (호스트: %s, endpoint: %s)", hostname, endpoint)
	log.Printf("수집 주기: 30초 | 서비스 체크: %d개", len(cfg.Checks))

	return func() {
		cancel()
		if err := provider.Shutdown(context.Background()); err != nil {
			log.Printf("MeterProvider 종료 실패: %v", err)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
