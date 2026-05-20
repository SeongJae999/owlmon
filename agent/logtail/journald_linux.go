//go:build linux

package logtail

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// JournaldCollector는 journalctl --output=json --follow 서브프로세스로
// systemd journal 로그를 실시간 수집합니다.
//
// Linux 전용 (다른 OS에서는 journald_other.go의 stub이 컴파일됨).
//
// 의존성: systemd가 설치된 Linux + journalctl 명령. CGO/sdjournal 사용 안 함.
type JournaldCollector struct {
	hostname        string
	source          string // 라벨 (예: "journald")
	includePatterns []string // 매칭 필터 (빈 배열이면 전부 송신 — 디스크 폭증 위험)
	pushFunc        func(LogEntry)
	mu              sync.Mutex
	cmd             *exec.Cmd
}

// NewJournaldCollector는 새 collector를 생성합니다.
// includePatterns가 비어있지 않으면, MESSAGE에 그 단어 중 하나라도 포함된 라인만 송신합니다.
// pushFunc는 LogEntry를 받아 Tailer 버퍼에 추가하는 콜백입니다.
func NewJournaldCollector(hostname, source string, includePatterns []string, pushFunc func(LogEntry)) *JournaldCollector {
	if source == "" {
		source = "journald"
	}
	return &JournaldCollector{
		hostname:        hostname,
		source:          source,
		includePatterns: includePatterns,
		pushFunc:        pushFunc,
	}
}

// Start는 journalctl --output=json --follow 서브프로세스를 시작합니다.
// ctx 취소 시 서브프로세스 정리 후 종료. 비정상 종료 시 5초 후 재시작.
func (c *JournaldCollector) Start(ctx context.Context) {
	go c.runLoop(ctx)
}

func (c *JournaldCollector) runLoop(ctx context.Context) {
	for {
		if err := c.run(ctx); err != nil {
			log.Printf("[journald] 서브프로세스 오류: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *JournaldCollector) run(ctx context.Context) error {
	// --no-pager: less 같은 페이저 비활성
	// --output=json: JSON Lines 형식
	// --follow: tail -f 처럼 새 로그만 스트림
	// --no-tail: 시작 시 기존 로그 안 출력 (현재 시점부터만)
	cmd := exec.CommandContext(ctx, "journalctl", "--no-pager", "--output=json", "--follow", "--no-tail")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.mu.Lock()
	c.cmd = cmd
	c.mu.Unlock()

	log.Println("[journald] 수집 시작")

	scanner := bufio.NewScanner(stdout)
	// journal 메시지가 길 수 있어 1MB까지 허용 (기본 64KB)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		raw := scanner.Bytes()
		entry, ok := parseJournaldLine(raw, c.hostname, c.source)
		if !ok {
			continue
		}
		// include_patterns 필터 — 매칭 안 되면 송신 X (DB 폭증 방지)
		if len(c.includePatterns) > 0 && !matchesAny(entry.Line, c.includePatterns) {
			continue
		}
		c.pushFunc(entry)
	}

	// 서브프로세스 종료 대기
	_ = cmd.Wait()

	c.mu.Lock()
	c.cmd = nil
	c.mu.Unlock()

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// parseJournaldLine는 journalctl JSON 한 줄을 LogEntry로 변환합니다.
// 필수 필드(MESSAGE) 없으면 ok=false.
func parseJournaldLine(raw []byte, hostname, source string) (LogEntry, bool) {
	var rec struct {
		RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
		Hostname          string `json:"_HOSTNAME"`
		SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
		Unit              string `json:"_SYSTEMD_UNIT"`
		Priority          string `json:"PRIORITY"`
		Message           string `json:"MESSAGE"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return LogEntry{}, false
	}
	if rec.Message == "" {
		return LogEntry{}, false
	}

	// __REALTIME_TIMESTAMP는 microsecond since epoch 문자열
	ts := time.Now()
	if rec.RealtimeTimestamp != "" {
		if us, err := strconv.ParseInt(rec.RealtimeTimestamp, 10, 64); err == nil {
			ts = time.Unix(0, us*1000) // µs → ns
		}
	}

	// host: journalctl 출력의 _HOSTNAME 우선, 없으면 agent hostname
	host := rec.Hostname
	if host == "" {
		host = hostname
	}

	// source 라벨은 collector 설정값 사용. file_path 자리에 SYSLOG_IDENTIFIER/_SYSTEMD_UNIT 표시
	filePath := rec.SyslogIdentifier
	if filePath == "" {
		filePath = rec.Unit
	}

	return LogEntry{
		Timestamp: ts,
		Host:      host,
		Source:    source,
		FilePath:  filePath,
		Line:      rec.Message,
		Level:     priorityToLevel(rec.Priority),
	}, true
}

// priorityToLevel는 syslog priority 숫자를 OWLmon 레벨 문자열로 변환합니다.
// syslog: 0=emerg, 1=alert, 2=crit, 3=err, 4=warning, 5=notice, 6=info, 7=debug
func priorityToLevel(p string) string {
	switch p {
	case "0", "1", "2":
		return "FATAL"
	case "3":
		return "ERROR"
	case "4":
		return "WARN"
	case "5", "6":
		return "INFO"
	case "7":
		return "DEBUG"
	default:
		return ""
	}
}
