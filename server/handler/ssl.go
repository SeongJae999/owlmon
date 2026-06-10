package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/ssl"
)

// SSLHandler는 SSL 인증서 모니터링 API를 처리합니다.
type SSLHandler struct {
	store   *db.SSLDomainStore
	checker *ssl.CertChecker
}

func NewSSLHandler(store *db.SSLDomainStore, checker *ssl.CertChecker) *SSLHandler {
	return &SSLHandler{store: store, checker: checker}
}

// ListDomains GET /api/ssl/domains
func (h *SSLHandler) ListDomains(w http.ResponseWriter, r *http.Request) {
	domains, err := h.store.List(context.Background())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	if domains == nil {
		domains = []db.SSLDomain{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(domains)
}

// AddDomain POST /api/ssl/domains
func (h *SSLHandler) AddDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
		Port   int    `json:"port"`
		Memo   string `json:"memo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if req.Domain == "" {
		http.Error(w, "도메인은 필수입니다", http.StatusBadRequest)
		return
	}
	if req.Port == 0 {
		req.Port = 443
	}

	domain, err := h.store.Add(context.Background(), db.SSLDomain{
		Domain: req.Domain,
		Port:   req.Port,
		Memo:   req.Memo,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}

	// 추가 후 즉시 체크
	go func() {
		domains, _ := h.store.List(context.Background())
		entries := domainsToEntries(domains)
		h.checker.CheckAll(entries)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(domain)
}

// UpdateDomain PATCH /api/ssl/domains/{id} — 메모만 수정
func (h *SSLHandler) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	var req struct {
		Memo string `json:"memo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateMemo(context.Background(), id, req.Memo); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteDomain DELETE /api/ssl/domains/{id}
func (h *SSLHandler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	if err := h.store.Delete(context.Background(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetStatus GET /api/ssl/status — 캐시된 인증서 상태 반환
func (h *SSLHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	results := h.checker.GetResults()
	if results == nil {
		results = []ssl.CertInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// TriggerCheck POST /api/ssl/check — 수동 즉시 체크
func (h *SSLHandler) TriggerCheck(w http.ResponseWriter, r *http.Request) {
	domains, err := h.store.List(context.Background())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	entries := domainsToEntries(domains)
	results := h.checker.CheckAll(entries)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func domainsToEntries(domains []db.SSLDomain) []ssl.DomainEntry {
	entries := make([]ssl.DomainEntry, len(domains))
	for i, d := range domains {
		entries[i] = ssl.DomainEntry{ID: d.ID, Domain: d.Domain, Port: d.Port}
	}
	return entries
}
