package logtail

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seongJae/owlmon/agent/config"
)

const (
	maxBufferSize = 10000 // 메모리 버퍼 최대 라인 수
	flushInterval = 10 * time.Second
)

// LogEntry는 수집된 로그 한 줄입니다.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Host      string    `json:"host"`
	Source    string    `json:"source"`
	FilePath  string    `json:"file_path"`
	Line      string    `json:"line"`
	Level     string    `json:"level"`
}

// Tailer는 로그 파일을 tail하여 서버로 전송합니다.
type Tailer struct {
	configs  []config.TailConfig
	hostname string
	sender   *Sender
	wal      *WAL
	mu       sync.Mutex
	buffer   []LogEntry
}

// NewTailer는 새 Tailer를 생성합니다.
// 기본적으로 디스크 영속화는 비활성. SetWALPath로 활성화하세요.
func NewTailer(configs []config.TailConfig, hostname, serverURL, agentKey string) *Tailer {
	return &Tailer{
		configs:  configs,
		hostname: hostname,
		sender:   NewSender(serverURL, agentKey),
		wal:      NewWAL(""),
	}
}

// SetWALPath는 디스크 영속화 경로를 설정합니다 (Start 호출 전에).
// 빈 문자열이면 영속화 비활성 (기본).
func (t *Tailer) SetWALPath(path string) {
	t.wal = NewWAL(path)
}

// Start는 WAL에서 미전송 로그 복원 후, 각 파일별 watcher와 flush 루프를 시작합니다.
func (t *Tailer) Start(ctx context.Context) {
	// WAL 복원 (있으면)
	if entries, err := t.wal.Load(); err != nil {
		log.Printf("로그 WAL 로드 실패: %v", err)
	} else if len(entries) > 0 {
		t.mu.Lock()
		t.buffer = append(entries, t.buffer...)
		t.mu.Unlock()
	}

	for _, cfg := range t.configs {
		go t.tailFile(ctx, cfg)
	}
	go t.flushLoop(ctx)
	log.Printf("로그 수집 시작: %d개 파일", len(t.configs))
}

// tailFile은 단일 파일을 fsnotify로 감시합니다.
// 파일 회전(rename/remove) 발생 시 watcher를 재설정합니다.
func (t *Tailer) tailFile(ctx context.Context, cfg config.TailConfig) {
	for {
		if err := t.watchFile(ctx, cfg); err != nil {
			log.Printf("로그 파일 감시 오류 (%s): %v", cfg.Path, err)
		}
		// 회전/삭제/오류 감지 시 5초 후 재시도
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// watchFile은 fsnotify로 파일 변경을 즉시 감지하여 새 라인을 읽습니다.
func (t *Tailer) watchFile(ctx context.Context, cfg config.TailConfig) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(cfg.Path); err != nil {
		return err
	}

	f, err := os.Open(cfg.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 끝으로 이동 (기존 내용 스킵)
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	reader := bufio.NewReader(f)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// 파일 회전/삭제 감지: 다시 열어야 함
			if ev.Op&(fsnotify.Rename|fsnotify.Remove) != 0 {
				return nil // 재시도 루프로 복귀
			}
			if ev.Op&fsnotify.Write != 0 {
				t.drainNewLines(reader, cfg)
			}
		case err := <-watcher.Errors:
			return err
		}
	}
}

// drainNewLines는 reader에서 가능한 모든 새 라인을 읽어 버퍼에 추가합니다.
func (t *Tailer) drainNewLines(reader *bufio.Reader, cfg config.TailConfig) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if len(cfg.IncludePatterns) > 0 && !matchesAny(line, cfg.IncludePatterns) {
			continue
		}

		entry := LogEntry{
			Timestamp: time.Now(),
			Host:      t.hostname,
			Source:    cfg.Name,
			FilePath:  cfg.Path,
			Line:      line,
			Level:     detectLevel(line),
		}

		t.mu.Lock()
		if len(t.buffer) < maxBufferSize {
			t.buffer = append(t.buffer, entry)
		}
		t.mu.Unlock()
	}
}

func (t *Tailer) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.flush()
			return
		case <-ticker.C:
			t.flush()
		}
	}
}

func (t *Tailer) flush() {
	t.mu.Lock()
	if len(t.buffer) == 0 {
		t.mu.Unlock()
		// WAL도 비워둠 (정리)
		_ = t.wal.Save(nil)
		return
	}
	entries := t.buffer
	t.buffer = nil
	t.mu.Unlock()

	if err := t.sender.Send(entries); err != nil {
		log.Printf("로그 전송 실패 (%d건): %v", len(entries), err)

		// 메모리에 다시 추가 + 디스크 WAL에도 저장
		t.mu.Lock()
		remaining := maxBufferSize - len(t.buffer)
		if remaining > 0 {
			if len(entries) > remaining {
				entries = entries[len(entries)-remaining:]
			}
			t.buffer = append(entries, t.buffer...)
		}
		bufCopy := append([]LogEntry(nil), t.buffer...)
		t.mu.Unlock()

		// WAL 저장은 락 밖에서 (디스크 I/O)
		if err := t.wal.Save(bufCopy); err != nil {
			log.Printf("로그 WAL 저장 실패: %v", err)
		}
		return
	}

	// 전송 성공 → WAL 비움
	_ = t.wal.Save(nil)
}

// matchesAny는 라인에 패턴 중 하나라도 포함되면 true입니다.
func matchesAny(line string, patterns []string) bool {
	upper := strings.ToUpper(line)
	for _, p := range patterns {
		if strings.Contains(upper, strings.ToUpper(p)) {
			return true
		}
	}
	return false
}

// detectLevel은 로그 라인에서 레벨을 자동 감지합니다.
func detectLevel(line string) string {
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "FATAL"):
		return "FATAL"
	case strings.Contains(upper, "ERROR"):
		return "ERROR"
	case strings.Contains(upper, "WARN"):
		return "WARN"
	case strings.Contains(upper, "INFO"):
		return "INFO"
	case strings.Contains(upper, "DEBUG"):
		return "DEBUG"
	default:
		return ""
	}
}
