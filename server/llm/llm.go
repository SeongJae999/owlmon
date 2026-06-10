// Package llm은 LLM(OpenAI/Ollama 등) 추상화 + 로그 설명·알림 요약 등 OWLmon용 인터페이스를 제공한다.
//
// 사용:
//   provider := llm.NewProvider() // OWLMON_LLM_PROVIDER 환경변수 기반
//   if provider != nil {
//       text, err := provider.Complete(ctx, prompt)
//   }
//
// 망분리 환경:
//   OWLMON_LLM_PROVIDER=ollama
//   OWLMON_OLLAMA_URL=http://192.168.0.30:11434
//   OWLMON_OLLAMA_MODEL=gemma4:12b
//
// 외부 API (학교/PoC):
//   OWLMON_LLM_PROVIDER=openai
//   OPENAI_API_KEY=sk-...
//   OWLMON_OPENAI_MODEL=gpt-4o-mini
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Provider는 LLM 호출을 추상화한다.
type Provider interface {
	// Complete는 단일 prompt에 대한 응답을 반환한다.
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)

	// Name은 provider 이름 (openai/ollama/mock).
	Name() string
}

// NewProvider는 환경변수 OWLMON_LLM_PROVIDER 기반으로 provider를 반환한다.
// 미설정 또는 "disabled"면 nil을 반환 (LLM 기능 비활성).
func NewProvider() Provider {
	switch strings.ToLower(os.Getenv("OWLMON_LLM_PROVIDER")) {
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil
		}
		return &OpenAIProvider{
			APIKey: key,
			Model:  envOr("OWLMON_OPENAI_MODEL", "gpt-4o-mini"),
			URL:    envOr("OWLMON_OPENAI_URL", "https://api.openai.com/v1/chat/completions"),
		}
	case "ollama":
		return &OllamaProvider{
			URL:   envOr("OWLMON_OLLAMA_URL", "http://localhost:11434"),
			Model: envOr("OWLMON_OLLAMA_MODEL", "gemma4:12b"),
		}
	case "mock":
		return &MockProvider{}
	}
	return nil
}

// Cache는 동일 입력에 대한 응답 캐싱 — 비용 절감 + 속도 향상.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

func NewCache(maxSize int, ttl time.Duration) *Cache {
	if maxSize <= 0 {
		maxSize = 500
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Cache{
		entries: make(map[string]cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}

func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 크기 제한 — 단순 FIFO 비슷한 정리 (만료된 것 우선 제거)
	if len(c.entries) >= c.maxSize {
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		// 그래도 크면 임의 하나 제거
		if len(c.entries) >= c.maxSize {
			for k := range c.entries {
				delete(c.entries, k)
				break
			}
		}
	}
	c.entries[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// MockProvider — 테스트/개발용. 실제 LLM 호출 없이 고정 응답.
type MockProvider struct{}

func (m *MockProvider) Name() string { return "mock" }
func (m *MockProvider) Complete(ctx context.Context, sys, user string) (string, error) {
	return fmt.Sprintf("(LLM 비활성 — MOCK 응답)\n입력 길이: %d자\n시스템: %s",
		len(user), strings.Split(sys, "\n")[0]), nil
}
