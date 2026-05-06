package logtail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Sender는 로그 엔트리를 서버에 HTTP로 전송합니다.
type Sender struct {
	serverURL string
	agentKey  string
	client    *http.Client
}

func NewSender(serverURL, agentKey string) *Sender {
	return &Sender{
		serverURL: serverURL,
		agentKey:  agentKey,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Send는 로그 엔트리 배치를 서버로 전송합니다.
func (s *Sender) Send(entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	// 배치당 최대 1000건
	if len(entries) > 1000 {
		entries = entries[:1000]
	}

	type wireEntry struct {
		Timestamp string `json:"timestamp"`
		Host      string `json:"host"`
		Source    string `json:"source"`
		FilePath  string `json:"file_path"`
		Line      string `json:"line"`
		Level     string `json:"level"`
	}

	payload := struct {
		Entries []wireEntry `json:"entries"`
	}{
		Entries: make([]wireEntry, len(entries)),
	}

	for i, e := range entries {
		payload.Entries[i] = wireEntry{
			Timestamp: e.Timestamp.Format(time.RFC3339Nano),
			Host:      e.Host,
			Source:    e.Source,
			FilePath:  e.FilePath,
			Line:      e.Line,
			Level:     e.Level,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON 직렬화 실패: %w", err)
	}

	req, err := http.NewRequest("POST", s.serverURL+"/api/logs/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("요청 생성 실패: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.agentKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("전송 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("서버 응답 오류: %d", resp.StatusCode)
	}
	return nil
}
