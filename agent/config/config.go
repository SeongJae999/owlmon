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

	return &cfg, nil
}
