package alert

import (
	"encoding/json"
	"errors"
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
	Channels     Channels `json:"channels"`      // 메일 외 통보 경로 (웹훅·문자·단계 통보)
}

// ErrChannelsNotSaved는 임계값·수신자는 저장됐지만 DB에 channels 칸이 없어 웹훅·문자 설정을 못 저장한 경우
var ErrChannelsNotSaved = errors.New("알림 경로 설정 칸 없음")

type Channels struct {
	Webhooks   []Webhook  `json:"webhooks"`
	SMS        SMSGateway `json:"sms"`
	Escalation Escalation `json:"escalation"`
}

// Webhook은 메신저·상위 관제로 보내는 HTTP POST 대상
type Webhook struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Format      string `json:"format"`       // "json"(기본) | "slack" — Slack·Mattermost·잔디 등 {"text"} 형식
	MinSeverity string `json:"min_severity"` // 이 등급 이상만 ("warning" | "critical")
	Recovery    bool   `json:"recovery"`     // 복구 알림도 보냄
}

// SMSGateway는 문자 업체·기관 내부 문자 서버의 HTTP API — 업체마다 형식이 달라 주소·헤더·본문 양식을 그대로 받음
type SMSGateway struct {
	Enabled     bool              `json:"enabled"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`       // 기본 POST
	ContentType string            `json:"content_type"` // 본문 형식 — json이면 값이 JSON 문자열로, form이면 URL 인코딩으로 들어감
	Headers     map[string]string `json:"headers"`      // 인증 키 등
	Body        string            `json:"body"`         // {{phone}} {{message}} 자리표시자
	Phones      []string          `json:"phones"`
	MinSeverity string            `json:"min_severity"` // 기본 critical
	Recovery    bool              `json:"recovery"`
}

// Escalation은 위험 알림을 정해진 시간 안에 아무도 확인하지 않으면 2차 담당자에게 다시 알리는 설정
type Escalation struct {
	AfterMin   int      `json:"after_min"` // 0이면 끔
	Recipients []string `json:"recipients"`
	Phones     []string `json:"phones"`
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
