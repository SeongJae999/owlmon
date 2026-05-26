package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config는 에이전트 전체 설정입니다.
type Config struct {
	OTLPEndpoint string        `yaml:"otlp_endpoint"` // OTel Collector 주소
	Checks       []CheckConfig `yaml:"checks"`        // 서비스 체크 목록
	Logs         LogConfig     `yaml:"logs"`          // 로그 수집 설정
	SNMP         SNMPConfig    `yaml:"snmp"`          // SNMP 프록시 폴링 (망분리 환경에서 OWLmon 서버 대신 폴링)
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
	IncludePatterns []string `yaml:"include_patterns"` // 빈 배열이면 main.go의 안전 기본값 사용
}

// TailConfig는 개별 로그 파일 수집 설정입니다.
type TailConfig struct {
	Name            string   `yaml:"name"`             // 로그 식별 이름
	Path            string   `yaml:"path"`             // 로그 파일 경로
	IncludePatterns []string `yaml:"include_patterns"` // 빈 배열이면 전부 수집
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
