package handler

import (
	"encoding/json"
	"net/http"

	"github.com/seongJae/owlmon/server/alert"
)

type AlertHandler struct {
	store   alert.ConfigStorer
	checker *alert.Checker     // nil이면 ack/유지보수 기능 비활성
	email   *alert.EmailConfig // SMTP 설정 상태 확인용
}

func NewAlertHandler(store alert.ConfigStorer, checker *alert.Checker, email *alert.EmailConfig) *AlertHandler {
	return &AlertHandler{store: store, checker: checker, email: email}
}

// GetEmailStatus는 SMTP 설정 + 수신자 등록 상태를 반환합니다.
// 미설정 상태면 UI 배너로 운영자에게 경고하기 위해 사용.
// GET /api/alert/email-status
func (h *AlertHandler) GetEmailStatus(w http.ResponseWriter, r *http.Request) {
	issues := []string{}
	smtpConfigured := h.email != nil && h.email.Host != "" && h.email.Username != "" && h.email.Password != ""
	if h.email == nil || h.email.Host == "" {
		issues = append(issues, "SMTP_HOST 미설정")
	}
	if h.email != nil && h.email.Host != "" && h.email.Username == "" {
		issues = append(issues, "SMTP_USERNAME 미설정")
	}
	if h.email != nil && h.email.Host != "" && h.email.Password == "" {
		issues = append(issues, "SMTP_PASSWORD 미설정")
	}

	recipientsCount := 0
	cfg := h.store.Get()
	recipientsCount = len(cfg.Recipients)
	if recipientsCount == 0 {
		issues = append(issues, "알림 수신자 등록 없음 (관리자 이메일)")
	}

	resp := map[string]interface{}{
		"smtp_configured":  smtpConfigured,
		"recipients_count": recipientsCount,
		"healthy":          smtpConfigured && recipientsCount > 0,
		"issues":           issues,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetConfig는 현재 알림 설정을 반환합니다.
func (h *AlertHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.store.Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// SetConfig는 알림 설정을 업데이트합니다.
func (h *AlertHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	var cfg alert.AlertConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	if err := h.store.Set(cfg); err != nil {
		http.Error(w, "설정 저장 실패", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// AckAlert는 알림을 확인 처리합니다. (이메일 재발송 억제)
// POST /api/alert/ack  body: {"host":"...", "category":"...", "severity":"..."}
func (h *AlertHandler) AckAlert(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		http.Error(w, "알림 체커 비활성", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Host     string `json:"host"`
		Category string `json:"category"`
		Severity string `json:"severity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	h.checker.Ack(req.Host, req.Category, req.Severity)
	w.WriteHeader(http.StatusNoContent)
}

// GetMaintenance는 유지보수 중인 호스트 목록을 반환합니다.
// GET /api/maintenance
func (h *AlertHandler) GetMaintenance(w http.ResponseWriter, r *http.Request) {
	var hosts []string
	if h.checker != nil {
		hosts = h.checker.MaintenanceHosts()
	} else {
		hosts = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hosts)
}

// SetMaintenance는 호스트의 유지보수 모드를 설정합니다.
// POST /api/maintenance  body: {"host":"...", "enabled":true}
func (h *AlertHandler) SetMaintenance(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		http.Error(w, "알림 체커 비활성", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Host    string `json:"host"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	h.checker.SetMaintenance(req.Host, req.Enabled)
	w.WriteHeader(http.StatusNoContent)
}
