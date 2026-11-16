package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seongJae/owlmon/server/alert"
)

type memAlertConfig struct{ c alert.AlertConfig }

func (m *memAlertConfig) Get() alert.AlertConfig        { return m.c }
func (m *memAlertConfig) Set(c alert.AlertConfig) error { m.c = c; return nil }

func TestSetConfigKeepsChannelsWhenAbsent(t *testing.T) {
	st := &memAlertConfig{alert.AlertConfig{Channels: alert.Channels{SMS: alert.SMSGateway{Enabled: true, URL: "http://sms", Phones: []string{"01011112222"}}}}}
	h := NewAlertHandler(st, nil, nil)
	post := func(body string) int {
		w := httptest.NewRecorder()
		h.SetConfig(w, httptest.NewRequest(http.MethodPost, "/api/alert/config", strings.NewReader(body)))
		return w.Code
	}
	if c := post(`{"enabled":true,"cpu_threshold":90}`); c != http.StatusOK || st.c.Channels.SMS.URL != "http://sms" || st.c.CPUThreshold != 90 {
		t.Fatalf("absent channels wiped: %d %+v", c, st.c.Channels)
	}
	if c := post(`{"enabled":true,"channels":{}}`); c != http.StatusOK || st.c.Channels.SMS.URL != "" {
		t.Fatalf("explicit channels not saved: %d %+v", c, st.c.Channels)
	}
	// 문자를 끈 채 2차 담당자 번호만으로는 단계 통보를 켤 수 없음
	if c := post(`{"channels":{"sms":{"url":"http://sms"},"escalation":{"after_min":10,"phones":["01011112222"]}}}`); c != http.StatusBadRequest {
		t.Fatalf("escalation without sms on: %d", c)
	}
	// 문자를 끄고 주소를 비워도 2차 번호를 남겨 둘 수 있음 (메일이 있으면 저장)
	if c := post(`{"channels":{"sms":{"enabled":false,"url":""},"escalation":{"after_min":10,"recipients":["boss@x.kr"],"phones":["01011112222"]}}}`); c != http.StatusOK {
		t.Fatalf("phones kept with sms off: %d", c)
	}
}

type keepAlertConfig struct {
	memAlertConfig
	kept int
}

func (k *keepAlertConfig) SetKeepChannels(c alert.AlertConfig) error {
	c.Channels = k.c.Channels
	k.c = c
	k.kept++
	return nil
}

func TestSetConfigSkipsChannelsWrite(t *testing.T) {
	st := &keepAlertConfig{memAlertConfig: memAlertConfig{alert.AlertConfig{Channels: alert.Channels{SMS: alert.SMSGateway{URL: "http://sms"}}}}}
	h := NewAlertHandler(st, nil, nil)
	w := httptest.NewRecorder()
	h.SetConfig(w, httptest.NewRequest(http.MethodPost, "/api/alert/config", strings.NewReader(`{"cpu_threshold":80}`)))
	if w.Code != http.StatusOK || st.kept != 1 || st.c.Channels.SMS.URL != "http://sms" || st.c.CPUThreshold != 80 {
		t.Fatalf("keep channels: %d kept=%d %+v", w.Code, st.kept, st.c)
	}
}
