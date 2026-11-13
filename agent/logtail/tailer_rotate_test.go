package logtail

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/seongJae/owlmon/agent/config"
)

// rename 회전(logrotate create) 직후 새 파일에 쓰인 줄은 재오픈 대기 중에 쓰였어도 읽혀야 함.
func TestTailFile_RotationReadsNewFileFromStart(t *testing.T) {
	retryDelay = 50 * time.Millisecond
	p := t.TempDir() + "/app.log"
	os.WriteFile(p, []byte("기존 내용\n"), 0644)

	tl := &Tailer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tl.tailFile(ctx, config.TailConfig{Path: p})
	time.Sleep(200 * time.Millisecond)

	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("before\n")
	f.Close()
	time.Sleep(100 * time.Millisecond)
	os.Rename(p, p+".1")
	os.WriteFile(p, []byte("after1\nafter2\n"), 0644)

	want := []string{"before", "after1", "after2"}
	var got []string
	for range 40 {
		time.Sleep(50 * time.Millisecond)
		tl.mu.Lock()
		got = got[:0]
		for _, e := range tl.buffer {
			got = append(got, e.Line)
		}
		tl.mu.Unlock()
		if slices.Equal(got, want) {
			return
		}
	}
	t.Fatalf("got %q, want %q", got, want)
}
