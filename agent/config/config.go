package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config는 에이전트 전체 설정입니다.
type Config struct {
	OTLPEndpoint string `yaml:"otlp_endpoint"` // 메트릭 받는 주소 (운영: 서버주소:443, 개발: Collector 직접)
	OTLPTLS      bool   `yaml:"otlp_tls"`      // 메트릭을 TLS로 보냄 (nginx 443). 예전 설치·개발은 false
	// 서버 추가 화면에서 받은 일회용 등록 토큰 — 처음 실행 때 서버별 키로 바꿔 agent.key에 저장 (그 뒤엔 안 씀)
	EnrollToken string `yaml:"enroll_token"`
	// 서버 인증서 sha256 지문 — 자체 서명 인증서일 때 이 인증서만 믿는다 (검증을 끄지 않고 고정). 비우면 시스템 CA로 검증
	ServerCertSHA256 string           `yaml:"server_cert_sha256"`
	Checks           []CheckConfig    `yaml:"checks"`      // 서비스 체크 목록
	Logs             LogConfig        `yaml:"logs"`        // 로그 수집 설정
	SNMP             SNMPConfig       `yaml:"snmp"`        // SNMP 프록시 폴링 (망분리 환경에서 OWLmon 서버 대신 폴링)
	SelfUpdate       SelfUpdateConfig `yaml:"self_update"` // agent 자가 업데이트
}

// SelfUpdateConfig는 agent 자가 업데이트 설정입니다.
// 망분리 환경에서 운영자가 일일이 sudo 명령 안 쳐도 되도록 — OWLmon 서버에서 자동 받음.
type SelfUpdateConfig struct {
	Enabled       bool          `yaml:"enabled"`        // 기본 false (opt-in)
	CheckInterval time.Duration `yaml:"check_interval"` // 기본 6h
	BinaryPath    string        `yaml:"binary_path"`    // 기본 /opt/owlmon/owlmon-agent
}

// SNMPConfig는 에이전트가 사내 SNMP 장비를 대신 폴링하는 설정입니다.
// 학교/공공기관 망분리 환경에서 OWLmon 서버가 직접 SNMP 못 닿을 때 사용.
type SNMPConfig struct {
	Enabled      bool          `yaml:"enabled"`
	PollInterval time.Duration `yaml:"poll_interval"` // 기본 30초
	Devices      []SNMPDevice  `yaml:"devices"`
}

// SNMPDevice는 폴링 대상 장비 1대 설정입니다.
type SNMPDevice struct {
	Name      string `yaml:"name"`      // 장비 식별 이름 (메트릭 라벨)
	IP        string `yaml:"ip"`        // 대상 IP
	Community string `yaml:"community"` // SNMP community (기본 public)
	Port      int    `yaml:"port"`      // UDP 포트 (기본 161)
	Type      string `yaml:"type"`      // "printer" / "switch" / "generic"
}

// LogConfig는 로그 수집 설정입니다.
type LogConfig struct {
	Enabled   bool           `yaml:"enabled"`
	ServerURL string         `yaml:"server_url"` // OWLmon 서버 주소
	AgentKey  string         `yaml:"agent_key"`  // 인증 키
	Tails     []TailConfig   `yaml:"tails"`
	WALPath   string         `yaml:"wal_path"` // 디스크 영속화 경로 (빈 문자열 = 비활성)
	Journald  JournaldConfig `yaml:"journald"` // systemd journal 수집 (Linux 전용)
}

// JournaldConfig는 systemd journal 수집 설정입니다.
type JournaldConfig struct {
	Enabled         bool     `yaml:"enabled"`          // 기본 false (opt-in)
	Source          string   `yaml:"source"`           // 라벨 (기본 "journald")
	IncludePatterns []string `yaml:"include_patterns"` // 빈 배열이면 main.go의 안전 기본값 사용. WARN 이상 레벨은 매칭 안 돼도 수집
}

// TailConfig는 개별 로그 파일 수집 설정입니다.
type TailConfig struct {
	Name            string   `yaml:"name"`             // 로그 식별 이름
	Path            string   `yaml:"path"`             // 로그 파일 경로
	IncludePatterns []string `yaml:"include_patterns"` // 빈 배열이면 전부 수집. WARN 이상 레벨은 매칭 안 돼도 수집
}

// CheckConfig는 개별 서비스 체크 설정입니다.
type CheckConfig struct {
	Name     string        `yaml:"name"`     // 체크 이름 (레이블로 사용)
	Type     string        `yaml:"type"`     // "http" 또는 "tcp"
	Target   string        `yaml:"target"`   // URL 또는 "host:port"
	Interval time.Duration `yaml:"interval"` // 체크 주기 (기본 60초)
}

// Load는 YAML 설정 파일을 읽어 Config를 반환합니다.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("설정 파일 읽기 실패 (%s): %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("설정 파일 파싱 실패: %w", err)
	}

	// 기본값 설정
	if cfg.OTLPEndpoint == "" {
		cfg.OTLPEndpoint = "localhost:4317"
	}
	for i := range cfg.Checks {
		if cfg.Checks[i].Interval == 0 {
			cfg.Checks[i].Interval = 60 * time.Second
		}
	}
	// SNMP 기본값
	if cfg.SNMP.PollInterval == 0 {
		cfg.SNMP.PollInterval = 30 * time.Second
	}
	for i := range cfg.SNMP.Devices {
		if cfg.SNMP.Devices[i].Community == "" {
			cfg.SNMP.Devices[i].Community = "public"
		}
		if cfg.SNMP.Devices[i].Port == 0 {
			cfg.SNMP.Devices[i].Port = 161
		}
		if cfg.SNMP.Devices[i].Type == "" {
			cfg.SNMP.Devices[i].Type = "generic"
		}
	}

	return &cfg, nil
}

// ── OWLmon 서버 연결 (등록·로그·스펙·self-update·메트릭 TLS 공용) ──

var (
	serverTLS       *tls.Config
	serverTransport http.RoundTripper = http.DefaultTransport
)

// PinServerCert — 서버 인증서를 지문으로 고정한다. 자체 서명이라 CA 검증은 못 하지만
// 지문이 다른 인증서(중간자)는 거부하므로 검증을 끄는 것(-k)보다 안전하다.
func PinServerCert(fingerprint string) error {
	want, err := hex.DecodeString(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(fingerprint)), ":", ""))
	if err != nil || len(want) != sha256.Size {
		return fmt.Errorf("server_cert_sha256 형식 오류 (sha256 16진수 64자): %q", fingerprint)
	}
	serverTLS = &tls.Config{
		InsecureSkipVerify: true, // CA 검증 대신 아래 지문 비교로 검증
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("서버 인증서 없음")
			}
			got := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(got[:], want) != 1 {
				return errors.New("서버 인증서 지문 불일치 — 설치 명령의 지문과 다른 인증서 (인증서를 바꿨다면 [서버 추가]로 다시 설치)")
			}
			return nil
		},
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = serverTLS
	serverTransport = t
	return nil
}

// ServerTLS — 메트릭 gRPC용. 지문 고정이 없으면 시스템 CA로 검증하는 기본 설정.
func ServerTLS() *tls.Config {
	if serverTLS == nil {
		return &tls.Config{}
	}
	return serverTLS
}

// ServerClient — OWLmon 서버로 가는 HTTP 요청용 (서비스 체크처럼 다른 사이트로 가는 요청엔 쓰지 말 것).
func ServerClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: serverTransport}
}
