package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seongJae/owlmon/server/db"
)

// fakeIngester는 LogIngester 인터페이스를 구현하는 테스트 더블입니다.
type fakeIngester struct {
	received []db.LogRecord
	err      error
}

func (f *fakeIngester) Enqueue(records []db.LogRecord) error {
	if f.err != nil {
		return f.err
	}
	f.received = append(f.received, records...)
	return nil
}

const validPayload = `{
  "resourceLogs": [{
    "resource": {
      "attributes": [{"key": "host.name", "value": {"stringValue": "test-host"}}]
    },
    "scopeLogs": [{
      "scope": {"name": "owlmon-agent"},
      "logRecords": [{
        "timeUnixNano": "1700000000000000000",
        "severityNumber": 9,
        "body": {"stringValue": "hello world"},
        "attributes": [{"key": "source", "value": {"stringValue": "journald"}}]
      }]
    }]
  }]
}`

func TestLogsHandler_ValidPayload_Returns200(t *testing.T) {
	ing := &fakeIngester{}
	h := NewLogsHandler(ing)

	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(validPayload))
	rec := httptest.NewRecorder()
	h.Receive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(ing.received) != 1 {
		t.Fatalf("expected 1 record, got %d", len(ing.received))
	}
	got := ing.received[0]
	if got.HostName != "test-host" {
		t.Errorf("host_name = %q, want %q", got.HostName, "test-host")
	}
	if got.Source != "journald" {
		t.Errorf("source = %q, want %q", got.Source, "journald")
	}
	if got.Severity != db.LogInfo {
		t.Errorf("severity = %d, want %d", got.Severity, db.LogInfo)
	}
	if got.Message != "hello world" {
		t.Errorf("message = %q, want %q", got.Message, "hello world")
	}
}

func TestLogsHandler_MissingHostName_Rejected(t *testing.T) {
	ing := &fakeIngester{}
	h := NewLogsHandler(ing)

	body := `{
		"resourceLogs": [{
			"resource": {"attributes": []},
			"scopeLogs": [{
				"scope": {"name": "test"},
				"logRecords": [{"body": {"stringValue": "hi"}}]
			}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Receive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(ing.received) != 0 {
		t.Errorf("expected 0 accepted, got %d", len(ing.received))
	}
	if !strings.Contains(rec.Body.String(), `"rejected":1`) {
		t.Errorf("expected rejected:1 in response, got %s", rec.Body.String())
	}
}

func TestLogsHandler_BadJSON_Returns400(t *testing.T) {
	h := NewLogsHandler(&fakeIngester{})
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(`{`))
	rec := httptest.NewRecorder()
	h.Receive(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLogsHandler_QueueFull_Returns503(t *testing.T) {
	ing := &fakeIngester{err: ErrIngestQueueFull}
	h := NewLogsHandler(ing)

	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(validPayload))
	rec := httptest.NewRecorder()
	h.Receive(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "1" {
		t.Errorf("Retry-After = %q, want %q", rec.Header().Get("Retry-After"), "1")
	}
}

func TestLogsHandler_OversizedBody_Returns413(t *testing.T) {
	h := NewLogsHandler(&fakeIngester{})
	// 11MB > 10MB 상한
	huge := strings.Repeat("a", 11*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(huge))
	rec := httptest.NewRecorder()
	h.Receive(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestLogsHandler_GenericIngesterError_Returns500(t *testing.T) {
	ing := &fakeIngester{err: errors.New("boom")}
	h := NewLogsHandler(ing)
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(validPayload))
	rec := httptest.NewRecorder()
	h.Receive(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
