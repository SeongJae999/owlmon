package logtail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

type ingestRecorder struct {
	mu    sync.Mutex
	lines []string
	fail  func(call int) bool
	calls int
}

func (r *ingestRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.fail != nil && r.fail(r.calls) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var p struct {
		Entries []struct {
			Line string `json:"line"`
		} `json:"entries"`
	}
	json.NewDecoder(req.Body).Decode(&p)
	for _, e := range p.Entries {
		r.lines = append(r.lines, e.Line)
	}
}

func fillBuffer(tl *Tailer, n int) {
	for i := range n {
		tl.buffer = append(tl.buffer, LogEntry{Line: strconv.Itoa(i)})
	}
}

// 1000건 넘는 버퍼도 한 번의 flush로 전부 전송돼야 함 (잘린 나머지가 성공 처리되며 사라지면 안 됨).
func TestFlush_SendsAllBeyondBatchLimit(t *testing.T) {
	rec := &ingestRecorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	tl := NewTailer(nil, "h", srv.URL, "k")
	fillBuffer(tl, 2500)
	tl.flush()

	if len(rec.lines) != 2500 {
		t.Fatalf("서버 수신 %d건, 2500건 기대", len(rec.lines))
	}
	if rec.lines[2499] != "2499" || len(tl.buffer) != 0 {
		t.Fatalf("마지막=%q 버퍼 잔여=%d", rec.lines[2499], len(tl.buffer))
	}
}

// 중간 배치가 실패하면 이미 보낸 배치는 다시 보내지 않고, 못 보낸 것만 버퍼에 남아야 함.
func TestFlush_PartialFailureKeepsUnsent(t *testing.T) {
	rec := &ingestRecorder{fail: func(call int) bool { return call == 2 }}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	tl := NewTailer(nil, "h", srv.URL, "k")
	fillBuffer(tl, 2500)
	tl.flush()

	if len(rec.lines) != 1000 {
		t.Fatalf("첫 배치만 수신돼야 함: %d건", len(rec.lines))
	}
	if len(tl.buffer) != 1500 || tl.buffer[0].Line != "1000" {
		t.Fatalf("미전송 1500건(1000번부터) 남아야 함: %d건", len(tl.buffer))
	}
}
