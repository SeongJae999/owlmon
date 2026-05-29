package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider — OpenAI Chat Completions API (또는 호환 endpoint).
type OpenAIProvider struct {
	APIKey string
	Model  string // gpt-4o-mini / gpt-4o / gpt-3.5-turbo 등
	URL    string // 기본 https://api.openai.com/v1/chat/completions
}

func (p *OpenAIProvider) Name() string { return "openai:" + p.Model }

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

type openaiResponse struct {
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := openaiRequest{
		Model: p.Model,
		Messages: []openaiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3, // 모니터링용은 일관성 우선
		MaxTokens:   800,
	}
	b, _ := json.Marshal(body)

	httpCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(httpCtx, "POST", p.URL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI 호출 실패: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var r openaiResponse
	if err := json.Unmarshal(rawBody, &r); err != nil {
		return "", fmt.Errorf("OpenAI 응답 파싱 실패: %w", err)
	}
	if r.Error != nil {
		return "", fmt.Errorf("OpenAI 오류: %s", r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("OpenAI 응답에 결과가 없습니다")
	}
	return r.Choices[0].Message.Content, nil
}
