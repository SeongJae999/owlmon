package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/auth"
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
	// 조회 전용 화면도 임계값은 필요하므로 API는 막지 않고 수신자 이메일만 뺌
	if auth.RoleFromContext(r.Context()) != auth.RoleAdmin {
		cfg.Recipients = []string{}
		cfg.Channels = alert.Channels{} // 문자 게이트웨이 인증 키·전화번호·웹훅 주소가 들어 있음
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// SetConfig는 알림 설정을 업데이트합니다.
func (h *AlertHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var cfg alert.AlertConfig
	var keys map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &cfg) != nil || json.Unmarshal(raw, &keys) != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	// channels를 모르는 요청(배포 전에 열어 둔 화면 등)은 문자 게이트웨이·웹훅 설정을 건드리지 않음.
	// 다시 읽어 채워 넣으면 읽기 실패(빈 값 반환)나 동시 저장 때 설정이 지워짐
	_, hasChannels := keys["channels"]
	keeper, canKeep := h.store.(interface{ SetKeepChannels(alert.AlertConfig) error })
	if !hasChannels && !canKeep {
		cfg.Channels = h.store.Get().Channels
	}
	if hasChannels || !canKeep {
		if msg := validateChannels(&cfg.Channels); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
	}
	save := h.store.Set
	if !hasChannels && canKeep {
		save = keeper.SetKeepChannels
	}
	if err := save(cfg); err != nil {
		msg := "설정 저장 실패"
		if errors.Is(err, alert.ErrChannelsNotSaved) {
			msg = "임계값·수신자는 저장했지만 웹훅·문자 설정은 저장하지 못했습니다 — DB에 설정 칸이 없습니다(서버 기동 로그 확인)"
		}
		http.Error(w, msg, http.StatusInternalServerError)
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
	h.checker.Ack(req.Host, req.Category, req.Severity, auth.UsernameFromContext(r.Context()))
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

// GET /api/alert/acks
func (h *AlertHandler) ListAcks(w http.ResponseWriter, r *http.Request) {
	type ack struct {
		Host     string    `json:"host"`
		Category string    `json:"category"`
		Severity string    `json:"severity"`
		AckedBy  string    `json:"acked_by"`
		AckedAt  time.Time `json:"acked_at"`
	}
	out := []ack{}
	if h.checker != nil {
		for _, a := range h.checker.Acks() {
			out = append(out, ack{a.Host, a.Category, a.Severity, a.By, a.At})
		}
	}
	writeJSON(w, out)
}

var phoneChars = regexp.MustCompile(`^[0-9+\-]{8,20}$`)

func httpURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https") && p.Host != ""
}

// validateChannels는 저장 전에 잘못된 값을 막음 — 틀린 주소로 저장되면 장애 때 알림이 조용히 안 나감
func validateChannels(c *alert.Channels) string {
	sev := map[string]bool{"": true, "warning": true, "critical": true}
	for i := range c.Webhooks {
		wh := &c.Webhooks[i]
		wh.URL = strings.TrimSpace(wh.URL)
		if !httpURL(wh.URL) {
			return "웹훅 주소는 http:// 또는 https://로 시작해야 합니다: " + wh.Name
		}
		if wh.Format != "" && wh.Format != "json" && wh.Format != "slack" {
			return "웹훅 형식은 json 또는 slack입니다"
		}
		if !sev[wh.MinSeverity] {
			return "웹훅 등급은 warning 또는 critical입니다"
		}
	}
	g := &c.SMS
	g.URL = strings.TrimSpace(g.URL)
	if g.Enabled && !httpURL(g.URL) {
		return "문자 게이트웨이 주소는 http:// 또는 https://로 시작해야 합니다"
	}
	if g.Method != "" && g.Method != http.MethodGet && g.Method != http.MethodPost && g.Method != http.MethodPut {
		return "문자 게이트웨이 방식은 GET·POST·PUT 중 하나입니다"
	}
	if !sev[g.MinSeverity] {
		return "문자 등급은 warning 또는 critical입니다"
	}
	for _, list := range [][]string{g.Phones, c.Escalation.Phones} {
		for _, p := range list {
			if !phoneChars.MatchString(p) {
				return "전화번호 형식이 아닙니다: " + p
			}
		}
	}
	if c.Escalation.AfterMin < 0 || c.Escalation.AfterMin > 1440 {
		return "단계 통보 시간은 0~1440분입니다"
	}
	// 문자 사용을 꺼 두면 2차 담당자 문자도 안 나가므로 받을 곳으로 치지 않음
	smsOn := g.Enabled && len(c.Escalation.Phones) > 0
	if c.Escalation.AfterMin > 0 && len(c.Escalation.Recipients) == 0 && !smsOn {
		return "단계 통보를 켜려면 2차 담당자 메일을 넣거나, 문자 사용을 켜고 2차 담당자 번호를 넣어 주세요"
	}
	return ""
}

// TestChannel POST /api/alert/test — 저장 전 화면의 값으로 바로 보내 봄 (관리자만)
func (h *AlertHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		http.Error(w, "알림 체커 비활성", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Channel string           `json:"channel"` // "webhook" | "sms"
		Webhook alert.Webhook    `json:"webhook"`
		SMS     alert.SMSGateway `json:"sms"`
		Phone   string           `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	m := alert.Notice{Severity: "warning", Subject: "[시험] OWLmon 알림 연결 확인", Body: "설정 화면에서 보낸 시험 알림입니다. 이 메시지가 보이면 연결이 정상입니다."}
	var err error
	switch req.Channel {
	case "webhook":
		if !httpURL(req.Webhook.URL) {
			http.Error(w, "웹훅 주소가 올바르지 않습니다", http.StatusBadRequest)
			return
		}
		err = h.checker.Notifier().SendWebhook(req.Webhook, m)
	case "sms":
		if !httpURL(req.SMS.URL) || !phoneChars.MatchString(req.Phone) {
			http.Error(w, "게이트웨이 주소나 전화번호가 올바르지 않습니다", http.StatusBadRequest)
			return
		}
		err = h.checker.Notifier().SendSMS(req.SMS, req.Phone, "[OWLmon] "+m.Subject)
	default:
		http.Error(w, "channel은 webhook 또는 sms입니다", http.StatusBadRequest)
		return
	}
	if err != nil {
		// 게이트웨이가 돌려준 문구는 원인 파악에 필요해 화면에 보여 줌. 주소는 경로·쿼리(키·번호)를 뺌
		msg := alert.RedactErr(err)
		var se *alert.StatusError
		if errors.As(err, &se) && se.Body != "" {
			msg += " — " + se.Body
		}
		log.Printf("알림 시험 발송 실패 (%s): %s", req.Channel, alert.RedactErr(err))
		http.Error(w, "보내지 못했습니다: "+msg, http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
