package logtail

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/seongJae/owlmon/agent/config"
)

// 개행 전에 읽힌 라인 앞부분이 다음 Write 때 이어붙어야 함 (유실 방지).
func TestDrainNewLines_PartialLineJoined(t *testing.T) {
	tl := &Tailer{}
	cfg := config.TailConfig{Name: "t", Path: "/x"}
	pending := tl.drainNewLines(bufio.NewReader(strings.NewReader("ERROR disk ")), cfg, "")
	if pending != "ERROR disk " || len(tl.buffer) != 0 {
		t.Fatalf("미완성 라인은 보류돼야 함: pending=%q buf=%d", pending, len(tl.buffer))
	}
	pending = tl.drainNewLines(bufio.NewReader(strings.NewReader("full\nnext")), cfg, pending)
	if len(tl.buffer) != 1 || tl.buffer[0].Line != "ERROR disk full" {
		t.Fatalf("이어붙인 라인 기대, got %+v", tl.buffer)
	}
	if pending != "next" {
		t.Fatalf("pending=%q", pending)
	}
}

// copytruncate 후 파일이 읽은 위치보다 작아지면 감지해야 함.
func TestTruncated_DetectsCopyTruncate(t *testing.T) {
	p := t.TempDir() + "/a.log"
	if err := os.WriteFile(p, []byte(strings.Repeat("x\n", 50)), 0644); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(p)
	defer f.Close()
	r := bufio.NewReader(f)
	io.ReadAll(r)
	if truncated(f, r) {
		t.Fatal("회전 전에는 false")
	}
	os.WriteFile(p, []byte("new\n"), 0644) // truncate 후 새로 씀
	if !truncated(f, r) {
		t.Fatal("copytruncate 감지 실패")
	}
}
