package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/db"
)

// 등록 토큰 유효기간 — 설치 명령을 받아 그날 안에 설치한다고 보고 24시간
const enrollTokenTTL = 24 * time.Hour

// 장비 이름은 Prometheus 라벨·gRPC 메타데이터(ASCII만 허용)로 그대로 쓰이므로 호스트 이름 문자만 받는다.
var agentNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type AgentHandler struct {
	store      *db.AgentStore
	audit      *audit.Store
	legacyKey  bool   // 공용 키 전환 설정이 켜져 있는지 (화면 경고용)
	certPath   string // nginx 인증서 (설치 명령의 지문 고정용)
	scriptsDir string // 설치 스크립트 폴더

	failMu sync.Mutex
	fails  map[string]*enrollFails // IP별 등록 실패 (무인증 enroll로 DB·감사 로그를 채우지 못하게)
}

type enrollFails struct {
	n     int
	since time.Time
}

// IP당 10분에 실패 10번이면 그 10분이 끝날 때까지 429 (DB·감사 기록 없이). 성공은 세지 않아 여러 대를 한꺼번에 설치해도 괜찮다
const (
	enrollMaxFailures = 10
	enrollFailWindow  = 10 * time.Minute
	maxTrackedIPs     = 1000
)

func NewAgentHandler(store *db.AgentStore, auditStore *audit.Store, legacyKey bool, certPath, scriptsDir string) *AgentHandler {
	return &AgentHandler{store: store, audit: auditStore, legacyKey: legacyKey, certPath: certPath, scriptsDir: scriptsDir,
		fails: make(map[string]*enrollFails)}
}

// enrollBlocked — 실패를 기록하려면 failed=true. 이미 한도를 넘었으면 true.
func (h *AgentHandler) enrollBlocked(ip string, failed bool) bool {
	h.failMu.Lock()
	defer h.failMu.Unlock()
	now := time.Now()
	f := h.fails[ip]
	if f != nil && now.Sub(f.since) > enrollFailWindow {
		delete(h.fails, ip)
		f = nil
	}
	if failed {
		if f == nil {
			if len(h.fails) >= maxTrackedIPs {
				for k, v := range h.fails {
					if now.Sub(v.since) > enrollFailWindow {
						delete(h.fails, k)
					}
				}
			}
			f = &enrollFails{since: now}
			h.fails[ip] = f
		}
		f.n++
	}
	return f != nil && f.n >= enrollMaxFailures
}

func randomHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IssueEnrollToken POST /api/agent/enroll-tokens — 관리자가 서버 이름으로 일회용 등록 토큰 발급
func (h *AgentHandler) IssueEnrollToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !agentNameRe.MatchString(req.Name) {
		http.Error(w, "서버 이름은 영문·숫자·점(.)·밑줄(_)·하이픈(-)으로 63자 이내", http.StatusBadRequest)
		return
	}
	token, err := randomHex()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	actor := ""
	if info, ok := audit.FromContext(r.Context()); ok {
		actor = info.Actor
	}
	expires, err := h.store.CreateEnrollToken(r.Context(), req.Name, token, actor, enrollTokenTTL)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	audit.Log(r.Context(), "agent.token.issue", "agent", req.Name, map[string]any{"expires_at": expires})
	writeJSON(w, map[string]any{"token": token, "name": req.Name, "expires_at": expires})
}

// Enroll POST /api/agent/enroll — 에이전트가 처음 실행될 때 등록 토큰을 자기 전용 키로 바꾼다 (무인증, 토큰이 곧 인증).
func (h *AgentHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	ip := audit.ClientIP(r)
	if h.enrollBlocked(ip, false) {
		http.Error(w, "등록 실패가 많아 잠시 막혔습니다. 10분 뒤 다시 시도하세요", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	key, err := randomHex()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	entry := audit.Entry{IP: ip, UserAgent: r.UserAgent(), Action: "agent.enroll", TargetType: "agent"}
	name, prev, err := h.store.Enroll(r.Context(), req.Token, key)
	if errors.Is(err, db.ErrTokenInvalid) {
		h.enrollBlocked(ip, true)
		entry.Actor, entry.Result = "anonymous", "unauthorized"
		h.audit.Save(r.Context(), entry)
		http.Error(w, "등록 토큰이 없거나 이미 쓰였거나 만료되었습니다 — 화면에서 새로 발급하세요", http.StatusUnauthorized)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	// prev: "" 새 장비, active 키 교체, blocked 폐기된 장비를 되살림
	entry.Actor, entry.TargetID, entry.Details = "agent:"+name, name, map[string]any{"prev_status": prev}
	h.audit.Save(r.Context(), entry)
	writeJSON(w, map[string]string{"key": key, "name": name})
}

type certInfo struct {
	SPKIPin    string `json:"spki_pin"`    // curl --pinnedpubkey sha256//<값>
	SHA256     string `json:"sha256"`      // 인증서 전체 sha256 (에이전트·PowerShell 고정용)
	SelfSigned bool   `json:"self_signed"` // 자체 서명이면 설치 명령에 지문 고정을 넣는다
}

// readCert — nginx가 쓰는 인증서를 읽어 지문을 만든다. 없으면(개발 환경) nil.
func readCert(path string) *certInfo {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	spki := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	whole := sha256.Sum256(c.Raw)
	return &certInfo{
		SPKIPin:    base64.StdEncoding.EncodeToString(spki[:]),
		SHA256:     hex.EncodeToString(whole[:]),
		SelfSigned: c.CheckSignatureFrom(c) == nil,
	}
}

// List GET /api/agents — 관리자용 에이전트 목록 + 서버 추가 화면에 필요한 정보
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	writeJSON(w, map[string]any{
		"agents":            agents,
		"legacy_shared_key": h.legacyKey,
		"cert":              readCert(h.certPath),
	})
}

// UpdateStatus POST /api/agents/{id}/status — 관리자용. blocked = 폐기 (즉시 메트릭·로그 거부)
func (h *AgentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 에이전트 ID", http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	switch req.Status {
	case "active", "blocked":
	default:
		http.Error(w, "status는 active/blocked 중 하나여야 합니다", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateStatus(r.Context(), id, req.Status); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	action := "agent.activate"
	if req.Status == "blocked" {
		action = "agent.revoke"
	}
	audit.Log(r.Context(), action, "agent", h.store.Name(r.Context(), id), nil)
	w.WriteHeader(http.StatusNoContent)
}

// Delete DELETE /api/agents/{id} — 관리자용 에이전트 삭제 (키도 함께 사라져 즉시 거부)
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 에이전트 ID", http.StatusBadRequest)
		return
	}
	name := h.store.Name(r.Context(), id)
	if err := h.store.Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	audit.Log(r.Context(), "agent.delete", "agent", name, nil)
	w.WriteHeader(http.StatusNoContent)
}

// InstallScript GET /api/agent/install-agent.sh, /api/agent/install-agent.ps1 — 무인증.
// 스크립트엔 비밀이 없고(토큰은 명령 인자로만 들어감), 아직 키가 없는 새 서버가 받아야 하므로.
func (h *AgentHandler) InstallScript(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join(h.scriptsDir, file))
		if err != nil {
			respondError(w, http.StatusNotFound, "설치 스크립트 없음", err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	}
}
