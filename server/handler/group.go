package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/db"
)

type GroupHandler struct {
	store *db.GroupStore
}

func NewGroupHandler(store *db.GroupStore) *GroupHandler {
	return &GroupHandler{store: store}
}

// GET /api/groups — 트리와 소속을 한 번에 (화면이 장비 상태와 합쳐 그룹별 수를 셈)
func (h *GroupHandler) Tree(w http.ResponseWriter, r *http.Request) {
	t, err := h.store.Tree(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	writeJSON(w, t)
}

func readGroup(w http.ResponseWriter, r *http.Request) (db.Group, bool) {
	var g db.Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return g, false
	}
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || utf8.RuneCountInString(g.Name) > 60 {
		http.Error(w, "그룹 이름은 1~60자로 입력하세요", http.StatusBadRequest)
		return g, false
	}
	r2, err := normalizeRanges(g.IPRanges)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return g, false
	}
	g.IPRanges = r2
	return g, true
}

// 화면이 다시 파싱하므로 세 가지 꼴로만 저장: 1.2.3.4 / 1.2.3.0/24 / 1.2.3.10-1.2.3.50 (IPv4만)
func normalizeRanges(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		n, err := normalizeRange(v)
		if err != nil {
			return nil, err
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if len(out) > db.MaxIPRanges {
		return nil, fmt.Errorf("IP 대역은 그룹당 %d개까지입니다", db.MaxIPRanges)
	}
	return out, nil
}

func normalizeRange(v string) (string, error) {
	bad := fmt.Errorf("IP 대역 형식이 아닙니다: %s (예: 192.168.0.0/24, 192.168.0.10-192.168.0.50)", v)
	v4 := func(s string) (netip.Addr, bool) {
		a, err := netip.ParseAddr(strings.TrimSpace(s))
		return a, err == nil && a.Is4()
	}
	if lo, hi, ok := strings.Cut(strings.ReplaceAll(v, "~", "-"), "-"); ok {
		a, ok1 := v4(lo)
		b, ok2 := v4(hi)
		if !ok1 || !ok2 || b.Less(a) {
			return "", bad
		}
		if a == b {
			return a.String(), nil
		}
		return a.String() + "-" + b.String(), nil
	}
	if strings.Contains(v, "/") {
		p, err := netip.ParsePrefix(v)
		if err != nil || !p.Addr().Is4() {
			return "", bad
		}
		return p.Masked().String(), nil
	}
	a, ok := v4(v)
	if !ok {
		return "", bad
	}
	return a.String(), nil
}

// POST /api/groups
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	g, ok := readGroup(w, r)
	if !ok {
		return
	}
	id, err := h.store.Create(r.Context(), g)
	if h.writeErr(w, err) {
		return
	}
	g.ID = id
	writeJSON(w, g)
}

// PUT /api/groups/{id} — 이름·순서·상위 그룹을 한 번에 (화면이 현재 값을 모두 보냄)
func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	g, ok := readGroup(w, r)
	if !ok {
		return
	}
	g.ID = id
	if !h.writeErr(w, h.store.Update(r.Context(), g)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// DELETE /api/groups/{id}
func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	if !h.writeErr(w, h.store.Delete(r.Context(), id)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// PUT /api/groups/members — {group_id: 숫자|null, items: [{kind, ref}]}. null이면 소속을 뺌
func (h *GroupHandler) Assign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID *int64           `json:"group_id"`
		Items   []db.GroupMember `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 5000 {
		http.Error(w, "장비를 1~5000개 고르세요", http.StatusBadRequest)
		return
	}
	for _, m := range req.Items {
		if !db.GroupKinds[m.Kind] || strings.TrimSpace(m.Ref) == "" {
			http.Error(w, "알 수 없는 장비입니다: "+m.Kind+" "+m.Ref, http.StatusBadRequest)
			return
		}
	}
	if !h.writeErr(w, h.store.Assign(r.Context(), req.GroupID, req.Items)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *GroupHandler) writeErr(w http.ResponseWriter, err error) bool {
	var gErr *db.GroupError
	switch {
	case err == nil:
		return false
	case errors.As(err, &gErr):
		http.Error(w, gErr.Msg, http.StatusConflict)
	default:
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
	}
	return true
}
