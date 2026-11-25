package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadOnlyGuardViewerWrites(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	guard := ReadOnlyGuard(ok)
	cases := []struct {
		role, method, path string
		want               int
	}{
		{RoleViewer, http.MethodPost, "/api/work-orders/12/status", http.StatusNoContent},
		{RoleViewer, http.MethodPost, "/api/alert/ack", http.StatusNoContent},
		{RoleViewer, http.MethodGet, "/api/work-orders", http.StatusNoContent},
		{RoleViewer, http.MethodPost, "/api/work-orders", http.StatusForbidden},
		{RoleViewer, http.MethodPut, "/api/work-orders/12", http.StatusForbidden},
		{RoleViewer, http.MethodDelete, "/api/work-orders/12", http.StatusForbidden},
		{RoleViewer, http.MethodPost, "/api/work-orders/12/status/x", http.StatusForbidden},
		{RoleViewer, http.MethodPost, "/api/work-orders/abc/status", http.StatusForbidden},
		{RoleAdmin, http.MethodDelete, "/api/work-orders/12", http.StatusNoContent},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		req = req.WithContext(context.WithValue(req.Context(), roleKey, c.role))
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s %s %s: %d, want %d", c.role, c.method, c.path, rec.Code, c.want)
		}
	}
}
