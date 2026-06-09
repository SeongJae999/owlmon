package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/seongJae/owlmon/server/loginsight"
)

// === Stub ===

type stubInsightStore struct {
	listItems    []loginsight.Insight
	listErr      error
	byTplResult  *loginsight.Insight
	byTplErr     error
	lastFilter   loginsight.Filter
	lastByTplKey string
}

func (s *stubInsightStore) List(ctx context.Context, f loginsight.Filter) ([]loginsight.Insight, error) {
	s.lastFilter = f
	return s.listItems, s.listErr
}

func (s *stubInsightStore) ByTemplate(ctx context.Context, hash string, maxAge time.Duration) (*loginsight.Insight, error) {
	s.lastByTplKey = hash
	return s.byTplResult, s.byTplErr
}

// === Status ===

func TestStatus_Enabled(t *testing.T) {
	h := NewInsightHandler(&stubInsightStore{})
	rr := httptest.NewRecorder()
	h.Status(rr, httptest.NewRequest("GET", "/api/insights/status", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp["enabled"] {
		t.Errorf("expected enabled=true, got %v", resp)
	}
}

func TestStatus_Disabled(t *testing.T) {
	h := NewInsightHandler(nil)
	rr := httptest.NewRecorder()
	h.Status(rr, httptest.NewRequest("GET", "/api/insights/status", nil))

	var resp map[string]bool
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["enabled"] {
		t.Errorf("expected enabled=false when store nil, got %v", resp)
	}
}

// === List ===

func TestList_DisabledReturns503(t *testing.T) {
	h := NewInsightHandler(nil)
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest("GET", "/api/insights/list", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestList_PassesFilterParams(t *testing.T) {
	store := &stubInsightStore{
		listItems: []loginsight.Insight{
			{ID: 1, SummaryKo: "test"},
		},
	}
	h := NewInsightHandler(store)

	from := "2026-05-10T00:00:00Z"
	to := "2026-05-11T00:00:00Z"
	url := "/api/insights/list?host=web1&severity=high&from=" + from + "&to=" + to + "&limit=25"
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest("GET", url, nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if store.lastFilter.HostName != "web1" {
		t.Errorf("HostName=%q", store.lastFilter.HostName)
	}
	if store.lastFilter.Severity != "high" {
		t.Errorf("Severity=%q", store.lastFilter.Severity)
	}
	if store.lastFilter.Limit != 25 {
		t.Errorf("Limit=%d", store.lastFilter.Limit)
	}
	if store.lastFilter.From.IsZero() {
		t.Error("From should be parsed")
	}
	if store.lastFilter.To.IsZero() {
		t.Error("To should be parsed")
	}
}

func TestList_InvalidDateIgnored(t *testing.T) {
	store := &stubInsightStore{}
	h := NewInsightHandler(store)
	url := "/api/insights/list?from=not-a-date"
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest("GET", url, nil))
	if !store.lastFilter.From.IsZero() {
		t.Errorf("invalid From should be ignored (zero), got %v", store.lastFilter.From)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (invalid date is silently ignored), got %d", rr.Code)
	}
}

func TestList_EmptyResultIsArray(t *testing.T) {
	// nil items가 JSON 출력에서 [] 로 나가야 (프론트 안전)
	store := &stubInsightStore{listItems: nil}
	h := NewInsightHandler(store)
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest("GET", "/api/insights/list", nil))

	body := rr.Body.String()
	if !strings.Contains(body, `"items":[]`) {
		t.Errorf("expected items:[], got %s", body)
	}
}

func TestList_StoreError(t *testing.T) {
	store := &stubInsightStore{listErr: errors.New("DB down")}
	h := NewInsightHandler(store)
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest("GET", "/api/insights/list", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// === ByTemplate ===

func TestByTemplate_Disabled503(t *testing.T) {
	h := NewInsightHandler(nil)
	rr := httptest.NewRecorder()
	h.ByTemplate(rr, httptest.NewRequest("GET", "/api/insights/by-template?hash=x", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestByTemplate_MissingHash400(t *testing.T) {
	h := NewInsightHandler(&stubInsightStore{})
	rr := httptest.NewRecorder()
	h.ByTemplate(rr, httptest.NewRequest("GET", "/api/insights/by-template", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestByTemplate_NotFound404(t *testing.T) {
	store := &stubInsightStore{byTplResult: nil}
	h := NewInsightHandler(store)
	rr := httptest.NewRecorder()
	h.ByTemplate(rr, httptest.NewRequest("GET", "/api/insights/by-template?hash=abc", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
	if store.lastByTplKey != "abc" {
		t.Errorf("hash not passed correctly: %s", store.lastByTplKey)
	}
}

func TestByTemplate_Found(t *testing.T) {
	store := &stubInsightStore{
		byTplResult: &loginsight.Insight{
			ID:          42,
			SummaryKo:   "디스크 부족",
			Severity:    loginsight.SeverityHigh,
			Category:    loginsight.CatDisk,
			ModelName:   "qwen2.5:7b",
		},
	}
	h := NewInsightHandler(store)
	rr := httptest.NewRecorder()
	h.ByTemplate(rr, httptest.NewRequest("GET", "/api/insights/by-template?hash=h", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"id":42`) {
		t.Errorf("missing id in body: %s", body)
	}
	if !strings.Contains(body, "디스크 부족") {
		t.Errorf("missing summary in body: %s", body)
	}
}

// === 인터페이스 호환성 ===

func TestStoreInterface(t *testing.T) {
	var _ insightStore = (*stubInsightStore)(nil)
	var _ insightStore = (*loginsight.Store)(nil)
}
