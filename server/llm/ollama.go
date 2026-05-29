package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider — Ollama 자체 호스팅 LLM (망분리 환경용).
//
// Ollama 설치 후 사용:
//   ollama pull qwen2.5:7b   # 한국어 좋음, 4-5GB
//   ollama pull llama3.1:8b  # 영어 강함, 5GB
//   ollama serve             # 11434 포트
type OllamaProvider struct {
	URL   string // 예: http://localhost:11434
	Model string // 예: qwen2.5:7b
}

func (p *OllamaProvider) Name() string { return "ollama:" + p.Model }

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
	Error   string        `json:"error,omitempty"`
}

func (p *OllamaProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := ollamaRequest{
		Model: p.Model,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
		Options: map[string]any{
			"temperature": 0.3,
			"num_predict": 800,
		},
	}
	b, _ := json.Marshal(body)

	httpCtx, cancel := context.WithTimeout(ctx, 60*time.Second) // 자체 호스팅은 느릴 수 있음
	defer cancel()
	url := strings.TrimRight(p.URL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(httpCtx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama 호출 실패 (URL %s): %w", url, err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var r ollamaResponse
	if err := json.Unmarshal(rawBody, &r); err != nil {
		return "", fmt.Errorf("Ollama 응답 파싱 실패: %w", err)
	}
	if r.Error != "" {
		return "", fmt.Errorf("Ollama 오류: %s", r.Error)
	}
	return r.Message.Content, nil
}
