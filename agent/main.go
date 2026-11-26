package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	neturl "net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/seongJae/owlmon/agent/collector"
	"github.com/seongJae/owlmon/agent/config"
	"github.com/seongJae/owlmon/agent/exporter"
	"github.com/seongJae/owlmon/agent/logtail"
	"github.com/seongJae/owlmon/agent/selfupdate"
	"github.com/seongJae/owlmon/agent/service"
	"github.com/seongJae/owlmon/agent/snmp"
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
	cfgPath := getEnv("OWLMON_CONFIG", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("설정 파일 로드 실패, 기본값 사용: %v", err)
		cfg = &config.Config{}
	}
	if cfg.ServerCertSHA256 != "" {
		if err := config.PinServerCert(cfg.ServerCertSHA256); err != nil {
			log.Fatalf("%v", err)
		}
	}
	serverURL := cfg.Logs.ServerURL
	if serverURL == "" {
		serverURL = getEnv("OWLMON_SERVER_URL", "http://localhost:8080")
	}

	endpoint := getEnv("OWLMON_OTLP_ENDPOINT", cfg.OTLPEndpoint)
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	agentKey := loadAgentKey(cfg, cfgPath, serverURL)
	if agentKey == "" {
		log.Println("agent_key 미설정 — 서버가 메트릭과 로그를 거부합니다")
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

	var otlpTLS *tls.Config
	if cfg.OTLPTLS {
		otlpTLS = config.ServerTLS()
	}
	otlpExp, err := exporter.NewOTLPExporter(ctx, endpoint, agentKey, otlpTLS)
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
	if err := collector.NewHostInfoCollector(ctx, meter, endpoint); err != nil {
		log.Fatalf("에이전트 정보 수집기 초기화 실패: %v", err)
	}

	// 로그 수집 시작 (파일 tail + journald 수집기는 같은 Tailer 버퍼 공유)
	if cfg.Logs.Enabled {
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
			patterns := cfg.Logs.Journald.IncludePatterns
			// 패턴 미지정 시 안전한 기본값 — DB 폭증 방지
			if len(patterns) == 0 {
				patterns = []string{"error", "fatal", "panic", "warning", "warn", "fail", "denied", "OOM", "kill"}
			}
			jc := logtail.NewJournaldCollector(hostname, cfg.Logs.Journald.Source, patterns, tailer.Push)
			jc.Start(ctx)
		}
	}

	// SNMP 프록시 폴링 (망분리 환경에서 사내 SNMP 장비를 에이전트가 대신 폴링)
	if cfg.SNMP.Enabled && len(cfg.SNMP.Devices) > 0 {
		devices := make([]snmp.Device, len(cfg.SNMP.Devices))
		for i, d := range cfg.SNMP.Devices {
			devices[i] = snmp.Device{
				Name:      d.Name,
				IP:        d.IP,
				Community: d.Community,
				Port:      d.Port,
				Type:      d.Type,
			}
		}
		poller := snmp.NewPoller(devices)
		if err := snmp.RegisterMetrics(meter, poller); err != nil {
			log.Printf("SNMP 메트릭 등록 실패: %v", err)
		} else {
			poller.Start(cfg.SNMP.PollInterval)
		}
	}

	// Self-update — opt-in (config.self_update.enabled: true 일 때만)
	if cfg.SelfUpdate.Enabled {
		selfupdate.Start(ctx, selfupdate.Config{
			Enabled:       true,
			ServerURL:     serverURL,
			CheckInterval: cfg.SelfUpdate.CheckInterval,
			BinaryPath:    cfg.SelfUpdate.BinaryPath,
			AgentKey:      agentKey,
		})
	}

	log.Printf("owlmon-agent 시작 (호스트: %s, endpoint: %s)", hostname, endpoint)
	log.Printf("수집 주기: %s | 서비스 체크: %d개 | SNMP 장비: %d개 | self-update: %v",
		collectInterval, len(cfg.Checks), len(cfg.SNMP.Devices), cfg.SelfUpdate.Enabled)

	// 스펙 1회 수집/전송 (백그라운드 — 실패해도 메트릭 송신엔 영향 없음)
	go sendSpecsOnce(ctx, serverURL, agentKey)

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
func sendSpecsOnce(ctx context.Context, serverURL, agentKey string) {
	s, err := specs.Collect()
	if err != nil {
		log.Printf("스펙 수집 실패: %v", err)
		return
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

// loadAgentKey — 우선순위: config의 agent_key(예전 설치) → agent.key(등록으로 받은 서버별 키)
// → 등록 토큰 교환(처음 실행) → OWLMON_AGENT_KEY 환경변수(예전 설치).
func loadAgentKey(cfg *config.Config, cfgPath, serverURL string) string {
	if cfg.Logs.AgentKey != "" {
		return cfg.Logs.AgentKey
	}
	keyFile := filepath.Join(filepath.Dir(cfgPath), "agent.key")
	if b, err := os.ReadFile(keyFile); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return string(bytes.TrimSpace(b))
	}
	if cfg.EnrollToken != "" {
		key, err := enroll(serverURL, cfg.EnrollToken)
		if err != nil {
			log.Fatalf("서버 등록 실패: %v", err)
		}
		if err := os.WriteFile(keyFile, []byte(key+"\n"), 0600); err != nil {
			log.Fatalf("키 저장 실패 (%s) — 토큰은 이미 쓰였으니 화면에서 새로 발급해 다시 설치하세요: %v", keyFile, err)
		}
		return key
	}
	return getEnv("OWLMON_AGENT_KEY", "")
}

// enroll — 일회용 등록 토큰을 이 서버 전용 키로 바꾼다. 연결 실패만 짧게 재시도
// (Windows 서비스는 시작 30초 안에 응답해야 하므로 길게 기다리지 않음 — 실패하면 종료해 서비스 재시작에 맡김).
func enroll(serverURL, token string) (string, error) {
	// 토큰과 응답으로 받는 키가 평문으로 오가지 않게 https만 (같은 PC의 localhost 시험은 예외)
	if u, err := neturl.Parse(serverURL); err != nil || (u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		return "", fmt.Errorf("등록은 https 서버 주소로만 합니다: %q", serverURL)
	}
	body, _ := json.Marshal(map[string]string{"token": token})
	url := strings.TrimRight(serverURL, "/") + "/api/agent/enroll"
	var lastErr error
	for i := range 3 {
		if i > 0 {
			time.Sleep(3 * time.Second)
		}
		resp, err := config.ServerClient(5*time.Second).Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
		}
		var out struct{ Key, Name string }
		if err := json.Unmarshal(msg, &out); err != nil || out.Key == "" {
			return "", fmt.Errorf("응답 해석 실패: %s", msg)
		}
		log.Printf("서버 등록 완료 — 장비 이름: %s", out.Name)
		return out.Key, nil
	}
	return "", lastErr
}
