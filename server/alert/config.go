package alert

import (
	"encoding/json"
	"os"
	"sync"
)

// AlertConfig는 알림 설정입니다. JSON 파일로 영구 저장됩니다.
type AlertConfig struct {
	Enabled      bool     `json:"enabled"`
	Recipients   []string `json:"recipients"`    // 수신자 이메일 목록
	CPUThreshold float64  `json:"cpu_threshold"` // CPU 위험 임계값 (%)
	MemThreshold float64  `json:"mem_threshold"` // 메모리 위험 임계값 (%)
	DiskWarn     float64  `json:"disk_warn"`     // 디스크 경고 임계값 (%)
	DiskCrit     float64  `json:"disk_crit"`     // 디스크 위험 임계값 (%)
}

func defaultConfig() *AlertConfig {
	return &AlertConfig{
		Enabled:      true,
		Recipients:   []string{},
		CPUThreshold: 90,
		MemThreshold: 95,
		DiskWarn:     85,
		DiskCrit:     90,
	}
}

// ConfigStorer는 알림 설정을 읽고 쓰는 인터페이스입니다.
type ConfigStorer interface {
	Get() AlertConfig
	Set(AlertConfig) error
}

// ConfigStore는 알림 설정을 파일로 저장하고 동시 접근을 관리합니다.
type ConfigStore struct {
	mu       sync.RWMutex
	config   *AlertConfig
	filePath string
}

func NewConfigStore(filePath string) *ConfigStore {
	s := &ConfigStore{filePath: filePath}
	s.config = s.load()
	return s
}

func (s *ConfigStore) Get() AlertConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := *s.config
	return c
}

func (s *ConfigStore) Set(cfg AlertConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = &cfg
	return s.save()
}

func (s *ConfigStore) load() *AlertConfig {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return defaultConfig()
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultConfig()
	}
	return cfg
}

func (s *ConfigStore) save() error {
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}
