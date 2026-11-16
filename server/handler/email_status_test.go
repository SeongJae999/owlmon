package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/seongJae/owlmon/server/alert"
)

func TestEmailStatusHealthy(t *testing.T) {
	smtp := &alert.EmailConfig{Host: "mail", Username: "u", Password: "p", To: []string{""}}
	healthy := func(cfg alert.AlertConfig, email *alert.EmailConfig) bool {
		w := httptest.NewRecorder()
		NewAlertHandler(&memAlertConfig{cfg}, nil, email).GetEmailStatus(w, httptest.NewRequest("GET", "/api/alert/email-status", nil))
		var out struct{ Healthy bool }
		json.NewDecoder(w.Body).Decode(&out)
		return out.Healthy
	}
	if healthy(alert.AlertConfig{}, nil) {
		t.Fatal("nothing configured but healthy")
	}
	if healthy(alert.AlertConfig{}, smtp) {
		t.Fatal("smtp without any recipient but healthy")
	}
	// 화면 수신자가 없어도 SMTP_TO로 실제 발송됨
	if !healthy(alert.AlertConfig{}, &alert.EmailConfig{Host: "mail", Username: "u", Password: "p", To: []string{"ops@x.kr"}}) {
		t.Fatal("SMTP_TO not counted")
	}
	if !healthy(alert.AlertConfig{Recipients: []string{"a@x.kr"}}, smtp) {
		t.Fatal("smtp + recipients not healthy")
	}
	// 메일 없이 웹훅만 써도 정상, 문자는 켜고 주소·번호가 있어야 정상
	if !healthy(alert.AlertConfig{Channels: alert.Channels{Webhooks: []alert.Webhook{{URL: "http://hook"}}}}, nil) {
		t.Fatal("webhook only not healthy")
	}
	if healthy(alert.AlertConfig{Channels: alert.Channels{SMS: alert.SMSGateway{Enabled: true, URL: "http://sms"}}}, nil) {
		t.Fatal("sms without phones healthy")
	}
}
