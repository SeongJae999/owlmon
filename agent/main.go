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
	"github.com/seongJae/owlmon/agent/logtail"
	"github.com/seongJae/owlmon/agent/service"
	"github.com/seongJae/owlmon/agent/specs"
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

	// 수집 주기 (환경변수 OWLMON_COLLECT_INTERVAL 우선, 기본 15초 — Prometheus/Datadog 표준선)
	// 형식: "15s", "1m" 같은 Go duration 문자열
	collectInterval := 15 * time.Second
	if v := os.Getenv("OWLMON_COLLECT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			collectInterval = d
		} else {
			log.Printf("OWLMON_COLLECT_INTERVAL 파싱 실패 (%q) — 기본 15초 사용", v)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	otlpExp, err := exporter.NewOTLPExporter(ctx, endpoint)
	if err != nil {
		log.Fatalf("exporter 초기화 실패: %v", err)
	}
	exp := exporter.NewBufferedExporter(otlpExp, "owlmon-buffer.json")

	hostname, _ := os.Hostname()
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("owlmon-agent"),
		semconv.ServiceVersion("0.1.0"),
		semconv.HostName(hostname),
	)

	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exp,
			metric.WithInterval(collectInterval),
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
	if _, err := collector.NewNetworkCollector(meter); err != nil {
		log.Fatalf("네트워크 수집기 초기화 실패: %v", err)
	}

	// 로그 수집 시작 (파일 tail + journald 수집기는 같은 Tailer 버퍼 공유)
	if cfg.Logs.Enabled {
		serverURL := cfg.Logs.ServerURL
		if serverURL == "" {
			serverURL = getEnv("OWLMON_SERVER_URL", "http://localhost:8080")
		}
		agentKey := cfg.Logs.AgentKey
		if agentKey == "" {
			agentKey = getEnv("OWLMON_AGENT_KEY", "")
		}

		tailer := logtail.NewTailer(cfg.Logs.Tails, hostname, serverURL, agentKey)
		if cfg.Logs.WALPath != "" {
			tailer.SetWALPath(cfg.Logs.WALPath)
		}
		tailer.Start(ctx)

		if len(cfg.Logs.Tails) > 0 {
			log.Printf("로그 파일 수집: %d개", len(cfg.Logs.Tails))
		}

		// journald 수집 (Linux 전용 — 다른 OS에서는 stub이 자동 비활성)
		if cfg.Logs.Journald.Enabled {
			jc := logtail.NewJournaldCollector(hostname, cfg.Logs.Journald.Source, tailer.Push)
			jc.Start(ctx)
		}
	}

	log.Printf("owlmon-agent 시작 (호스트: %s, endpoint: %s)", hostname, endpoint)
	log.Printf("수집 주기: %s | 서비스 체크: %d개", collectInterval, len(cfg.Checks))

	// 스펙 1회 수집/전송 (백그라운드 — 실패해도 메트릭 송신엔 영향 없음)
	go sendSpecsOnce(ctx, cfg)

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

// sendSpecsOnce는 호스트 스펙을 1회 수집해서 OWLmon 서버로 전송한다.
// 실패해도 에이전트의 메트릭 송신엔 영향 없도록 별도 goroutine에서 호출한다.
func sendSpecsOnce(ctx context.Context, cfg *config.Config) {
	s, err := specs.Collect()
	if err != nil {
		log.Printf("스펙 수집 실패: %v", err)
		return
	}

	serverURL := cfg.Logs.ServerURL
	if serverURL == "" {
		serverURL = getEnv("OWLMON_SERVER_URL", "http://localhost:8080")
	}
	agentKey := cfg.Logs.AgentKey
	if agentKey == "" {
		agentKey = getEnv("OWLMON_AGENT_KEY", "")
	}

	// 송신 타임아웃 분리
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := s.Send(sendCtx, serverURL, agentKey); err != nil {
		log.Printf("스펙 전송 실패 (메트릭 송신엔 영향 없음): %v", err)
		return
	}
	log.Printf("호스트 스펙 전송 완료: CPU=%s(%d코어), RAM=%dGB, 디스크=%d개",
		s.CPU.Model, s.CPU.Cores, s.MemoryTotalBytes/(1<<30), len(s.Disks))
}
