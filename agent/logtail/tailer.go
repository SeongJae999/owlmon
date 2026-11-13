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
	dropped  int // 버퍼가 가득 차 버린 라인 수 (flush 때 로그로 알림)
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

// Push는 외부 수집기(journald 등)가 엔트리를 버퍼에 추가합니다.
// 버퍼가 가득 차면 드롭하고 dropped로 집계합니다 (다음 flush 때 경고).
func (t *Tailer) Push(entry LogEntry) {
	t.mu.Lock()
	if len(t.buffer) < maxBufferSize {
		t.buffer = append(t.buffer, entry)
	} else {
		t.dropped++
	}
	t.mu.Unlock()
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
	var pending string // 개행 전까지 쓰인 미완성 라인 (다음 Write 때 이어붙임)

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
				// copytruncate 회전: 파일이 읽은 위치보다 작아지면 처음부터 다시 읽음
				if truncated(f, reader) {
					if _, err := f.Seek(0, io.SeekStart); err != nil {
						return err
					}
					reader.Reset(f)
					pending = ""
				}
				pending = t.drainNewLines(reader, cfg, pending)
			}
		case err := <-watcher.Errors:
			return err
		}
	}
}

const maxPendingLine = 64 << 10

// truncated는 파일 크기가 현재 읽기 위치보다 작아졌는지(copytruncate) 확인합니다.
func truncated(f *os.File, reader *bufio.Reader) bool {
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Size() < pos-int64(reader.Buffered())
}

// drainNewLines는 reader에서 가능한 모든 새 라인을 읽어 버퍼에 추가하고,
// 개행 없이 끝난 미완성 라인을 반환합니다 (버리면 라인 앞부분이 유실됨).
func (t *Tailer) drainNewLines(reader *bufio.Reader, cfg config.TailConfig, pending string) string {
	for {
		chunk, err := reader.ReadString('\n')
		line := pending + chunk
		pending = ""
		if err != nil {
			if len(line) < maxPendingLine {
				return line
			}
			// 개행 없이 계속 쌓이는 경우(바이너리 등) 무한 증가 방지 — 한 줄로 확정
		}
		// UTF-16LE 로그는 0x00이 섞여 레벨 감지가 안 되고 서버 저장도 실패함 — ASCII 부분이라도 살림
		line = strings.TrimRight(strings.ReplaceAll(line, "\x00", ""), "\r\n")
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
		} else {
			t.dropped++
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
	dropped := t.dropped
	t.dropped = 0
	t.mu.Unlock()
	if dropped > 0 {
		log.Printf("⚠️ 로그 버퍼 가득 참 — %d건 유실 (서버 전송 지연 또는 로그 폭주)", dropped)
	}

	if sent, err := t.sender.Send(entries); err != nil {
		entries = entries[sent:]
		log.Printf("로그 전송 실패 (%d건 미전송): %v", len(entries), err)

		// 메모리에 다시 추가 + 디스크 WAL에도 저장
		t.mu.Lock()
		if remaining := maxBufferSize - len(t.buffer); len(entries) > remaining {
			t.dropped += len(entries) - remaining
			entries = entries[len(entries)-remaining:]
		}
		t.buffer = append(entries, t.buffer...)
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
