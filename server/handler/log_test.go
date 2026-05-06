package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// 라벨링 핸들러 검증 테스트.
// store는 nil로 둬서 503/400/잘못된 id 같은 진입 단계 검증만 다룬다.
// 전체 흐름(DB 포함) 테스트는 Phase 0 Week 6 통합 테스트에서 처리.

// newTestHandler는 Store가 모두 nil인 핸들러를 만든다.
// (annStore=nil → 라벨링 메서드는 503 반환해야 함)
func newTestHandler() *LogHandler {
	return NewLogHandler(nil, nil, nil, "")
}

func mountAnnotateRouter(h *LogHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/logs/{id}", h.GetByID)
	r.Post("/api/logs/{id}/annotate", h.Annotate)
	r.Get("/api/logs/{id}/annotations", h.ListAnnotationsByLog)
	r.Get("/api/logs/annotations", h.ListAnnotations)
	r.Delete("/api/logs/annotations/{id}", h.DeleteAnnotation)
	return r
}

// --- annStore nil 일 때 모든 라벨링 메서드는 503을 반환해야 한다 ---

func TestAnnotate_AnnStoreNil_503(t *testing.T) {
	h := newTestHandler()
	srv := mountAnnotateRouter(h)

	body, _ := json.Marshal(map[string]any{"problem": "x", "solution": "y"})
	req := httptest.NewRequest(http.MethodPost, "/api/logs/1/annotate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListAnnotationsByLog_AnnStoreNil_503(t *testing.T) {
	h := newTestHandler()
	srv := mountAnnotateRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/logs/1/annotations", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestListAnnotations_AnnStoreNil_503(t *testing.T) {
	h := newTestHandler()
	srv := mountAnnotateRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/logs/annotations", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestDeleteAnnotation_AnnStoreNil_503(t *testing.T) {
	h := newTestHandler()
	srv := mountAnnotateRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/logs/annotations/5", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// --- 잘못된 id 형식 → 400 ---

func TestAnnotate_BadID_400(t *testing.T) {
	// annStore가 nil이면 503이 먼저 나오므로 이 테스트는 nil이 아닌 store 필요.
	// 단, store nil이라도 chi 라우팅 단에서 path 파라미터 파싱 후 503보다 먼저 검증됨.
	// 현재 코드는 nil 체크가 id 파싱보다 앞 → 503이 우선.
	// 따라서 별도 테스트로 강제로는 검증하기 어렵고, GetByID에서만 가능하다.
	t.Skip("annStore nil 가드가 id 파싱보다 앞이라 직접 검증 불가 — DB 통합 테스트에서 다룸")
}

func TestGetByID_BadID_400(t *testing.T) {
	h := newTestHandler()
	srv := mountAnnotateRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/logs/abc", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteAnnotation_BadID_returns503BeforeIDCheck(t *testing.T) {
	// annStore nil 가드가 id 파싱보다 우선이라 503이 먼저 떨어진다.
	// 이 동작이 의도된 것임을 명시하는 회귀 테스트.
	h := newTestHandler()
	srv := mountAnnotateRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/logs/annotations/abc", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (annStore nil first), got %d", rec.Code)
	}
}
