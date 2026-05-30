package loginsight

import (
	"os"
	"strconv"
	"time"
)

// Config — 워커 동작 설정. 환경변수에서 LoadConfig()로 로드.
//
// 환경변수 (모두 선택, 미설정 시 기본값):
//
//	OWLMON_LOGINSIGHT_ENABLED           true/false      기본 false
//	OWLMON_LOGINSIGHT_INTERVAL          5m / 30s / 1h   기본 5m
//	OWLMON_LOGINSIGHT_WINDOW            매 tick fetch 범위 기본 5m
//	OWLMON_LOGINSIGHT_PER_HOST_PER_MIN  호스트당 분당 그룹 한도 기본 10
//	OWLMON_LOGINSIGHT_CACHE_TTL         템플릿 캐시 TTL 기본 24h
//	OWLMON_LOGINSIGHT_MAX_LLM_PER_TICK  tick당 최대 LLM 호출 기본 20
type Config struct {
	Enabled            bool
	Interval           time.Duration
	Window             time.Duration
	SeverityFilter     []string
	PerHostPerMin      int
	SamplesPerGroup    int
	CacheTTL           time.Duration
	MaxLLMCallsPerTick int
}

// LoadConfig — 환경변수 기반으로 Config 생성. 미설정값은 기본값.
func LoadConfig() Config {
	return Config{
		Enabled:            envBool("OWLMON_LOGINSIGHT_ENABLED", false),
		Interval:           envDuration("OWLMON_LOGINSIGHT_INTERVAL", 5*time.Minute),
		Window:             envDuration("OWLMON_LOGINSIGHT_WINDOW", 5*time.Minute),
		SeverityFilter:     []string{"error", "warn", "warning", "fatal", "critical"},
		PerHostPerMin:      envInt("OWLMON_LOGINSIGHT_PER_HOST_PER_MIN", 10),
		SamplesPerGroup:    3,
		CacheTTL:           envDuration("OWLMON_LOGINSIGHT_CACHE_TTL", 24*time.Hour),
		MaxLLMCallsPerTick: envInt("OWLMON_LOGINSIGHT_MAX_LLM_PER_TICK", 20),
	}
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
