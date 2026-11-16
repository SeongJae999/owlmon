package alert

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRenderBodyEscapes(t *testing.T) {
	vars := map[string]string{"phone": "010-1234-5678", "message": `CPU "96%" & 다운`}
	j := renderBody(`{"to":"{{phone}}","text":"{{message}}"}`, "application/json", vars)
	if j != `{"to":"010-1234-5678","text":"CPU \"96%\" & 다운"}` {
		t.Errorf("json: %s", j)
	}
	f := renderBody(`to={{phone}}&msg={{message}}`, "application/x-www-form-urlencoded", vars)
	if f != "to=010-1234-5678&msg=CPU+%2296%25%22+%26+%EB%8B%A4%EC%9A%B4" {
		t.Errorf("form: %s", f)
	}
}

func TestRouteNotice(t *testing.T) {
	cfg := AlertConfig{Recipients: []string{"ops@x.kr"}, Channels: Channels{
		Webhooks: []Webhook{{Name: "all", URL: "http://a"}, {Name: "crit", URL: "http://b", MinSeverity: "critical", Recovery: true}},
		SMS:      SMSGateway{Enabled: true, URL: "http://sms", Phones: []string{"01011112222"}},
	}}
	w := routeNotice(cfg, nil, "warning", "[주의] web1 디스크", "")
	if len(w.Webhooks) != 1 || w.Webhooks[0].Name != "all" || len(w.Phones) != 0 || w.Emails[0] != "ops@x.kr" {
		t.Errorf("warning: %+v", w)
	}
	c := routeNotice(cfg, nil, "critical", "[위험] web1 다운", "")
	if len(c.Webhooks) != 2 || len(c.Phones) != 1 {
		t.Errorf("critical: %+v", c)
	}
	r := routeNotice(cfg, nil, "info", "[복구] web1 복구", "")
	if len(r.Webhooks) != 1 || r.Webhooks[0].Name != "crit" || len(r.Phones) != 0 {
		t.Errorf("recovery: %+v", r)
	}
	cfg.Channels.SMS.Enabled = false
	if p := routeNotice(cfg, nil, "critical", "x", "").Phones; len(p) != 0 {
		t.Errorf("sms disabled: %v", p)
	}
	if e := routeNotice(AlertConfig{}, []string{"env@x.kr"}, "critical", "x", "").Emails; len(e) != 1 || e[0] != "env@x.kr" {
		t.Errorf("default recipients: %v", e)
	}
}

func TestSendSMSAndWebhook(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.RawQuery+" "+r.Header.Get("X-Key")+" "+string(b))
		if strings.Contains(string(b), "fail") {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("bad key"))
		}
	}))
	defer srv.Close()
	n := NewNotifier(nil)
	g := SMSGateway{URL: srv.URL, Headers: map[string]string{"X-Key": "k1"}, Body: `{"to":"{{phone}}","msg":"{{message}}"}`}
	if err := n.SendSMS(g, "01011112222", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := n.SendSMS(SMSGateway{URL: srv.URL, Body: "fail"}, "010", "x"); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("want gateway error, got %v", err)
	}
	get := SMSGateway{URL: srv.URL + "?to={{phone}}&m={{message}}", Method: "GET"}
	if err := n.SendSMS(get, "01011112222", "a b"); err != nil {
		t.Fatal(err)
	}
	if err := n.SendWebhook(Webhook{URL: srv.URL, Format: "slack"}, Notice{Subject: "s", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`POST  k1 {"to":"01011112222","msg":"hello"}`,
		`POST   fail`,
		`GET to=01011112222&m=a+b  `,
		`POST   {"text":"*[OWLmon] s*\nb"}`,
	}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("req %d = %q, want %q", i, safe(got, i), w)
		}
	}
}

func safe(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "<none>"
}

type memConfig struct{ c AlertConfig }

func (m *memConfig) Get() AlertConfig        { return m.c }
func (m *memConfig) Set(c AlertConfig) error { m.c = c; return nil }

func drain(n *Notifier) []Notice {
	var out []Notice
	for {
		m, ok := n.mail.pop()
		if !ok {
			return out
		}
		out = append(out, m)
	}
}

type memHistory struct{ subjects []string }

func (h *memHistory) Save(_ context.Context, _, _, _, subject, _ string) error {
	h.subjects = append(h.subjects, subject)
	return nil
}

func newTestChecker(cfg AlertConfig) (*Checker, *memConfig) {
	mc := &memConfig{cfg}
	c := NewChecker("", &EmailConfig{To: []string{"ops@x.kr"}}, mc, nil)
	return c, mc
}

func escCfg(after int) AlertConfig {
	return AlertConfig{Enabled: true, Channels: Channels{Escalation: Escalation{AfterMin: after, Recipients: []string{"boss@x.kr"}}}}
}

func TestEscalation(t *testing.T) {
	cfg := escCfg(10)
	c, _ := newTestChecker(cfg)
	c.SendAlert("web1", "down", "critical", "[위험] web1 다운", "호스트: web1")
	c.SendAlert("db1", "down", "critical", "[위험] db1 다운", "호스트: db1")
	c.SendAlert("web2", "disk", "warning", "[주의] web2 디스크", "")
	c.SendAlert("web5", "anomaly_cpu", "critical", "[이상] web5", "") // 화면에서 확인할 수 없는 항목은 대상 아님
	drain(c.notifier)

	now := time.Now()
	c.escalate(cfg, now.Add(5*time.Minute))
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("too early: %d", len(got))
	}
	c.Ack("db1", "down", "critical", "kim")                 // 확인한 건은 올리지 않음
	c.SendAlert("web9", "down", "info", "[복구] web9 복구", "") // 다른 대상 복구는 영향 없음
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	got := drain(c.notifier)
	if len(got) != 1 || !strings.HasPrefix(got[0].Subject, "[미확인 10분] [위험] web1") || got[0].Emails[0] != "boss@x.kr" {
		t.Fatalf("escalation: %+v", got)
	}
	c.escalate(cfg, now.Add(20*time.Minute)) // 한 번만
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("repeated: %d", len(got))
	}

	// 복구되면 대기에서 빠짐
	c.SendAlert("web3", "down", "critical", "[위험] web3 다운", "")
	c.SendAlert("web3", "down", "info", "[복구] web3 복구", "")
	drain(c.notifier)
	c.escalate(cfg, now.Add(30*time.Minute))
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("recovered escalated: %+v", got)
	}
}

func TestEscalationEdges(t *testing.T) {
	now := time.Now()

	// 최댓값 1440분도 TTL에 먼저 걸려 사라지지 않음
	cfg := escCfg(1440)
	c, _ := newTestChecker(cfg)
	c.SendAlert("web1", "down", "critical", "[위험] web1 다운", "")
	drain(c.notifier)
	c.escalate(cfg, now.Add(1440*time.Minute+30*time.Second))
	if got := drain(c.notifier); len(got) != 1 {
		t.Fatalf("1440min: %d", len(got))
	}

	// 디스크: 마운트별 대기 — 한 마운트 복구가 다른 마운트 대기를 지우지 않음
	cfg = escCfg(10)
	c, _ = newTestChecker(cfg)
	c.send("web1", "disk", "/", "critical", "[위험] web1 디스크 /", "")
	c.send("web1", "disk", "/data", "critical", "[위험] web1 디스크 /data", "")
	c.send("web1", "disk", "/", "info", "[복구] web1 디스크 /", "")
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "/data") {
		t.Fatalf("disk target: %+v", got)
	}

	// 다른 마운트의 주의를 확인해도 위험 대기는 남고, 위험을 확인하면 모든 마운트의 대기가 끝남
	c, _ = newTestChecker(cfg)
	c.send("web1", "disk", "/", "critical", "[위험] web1 디스크 /", "")
	c.Ack("web1", "disk", "warning", "kim")
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 1 {
		t.Fatalf("warning ack dropped critical: %+v", got)
	}
	c, _ = newTestChecker(cfg)
	c.send("web1", "disk", "/", "critical", "[위험] web1 디스크 /", "")
	c.send("web1", "disk", "/data", "critical", "[위험] web1 디스크 /data", "")
	c.Ack("web1", "disk", "critical", "kim")
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("critical ack: %+v", got)
	}

	// 장애 중에 감시를 끄거나 지운 대상은 2차 통보하지 않음
	c, _ = newTestChecker(cfg)
	c.SetWatched("synthetic", func(context.Context) (map[string]bool, error) { return map[string]bool{"api": true}, nil })
	c.SendAlert("shop", "synthetic", "critical", "[위험] shop", "")
	c.SendAlert("api", "synthetic", "critical", "[위험] api", "")
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "api") || len(c.unacked) != 1 {
		t.Fatalf("watched: %+v", got)
	}

	// 점검 모드로 들어가면 대기를 버림 — 점검 해제 뒤 지난 장애가 올라가지 않음
	c, _ = newTestChecker(cfg)
	c.SendAlert("web1", "down", "critical", "[위험] web1 다운", "")
	c.SetMaintenance("web1", true)
	c.SetMaintenance("web1", false)
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("maintenance: %+v", got)
	}

	// 서버가 끊긴 동안엔 CPU 단계 통보를 미루고(대기 유지), 돌아왔는데 아직 확인 전이면 통보
	c, _ = newTestChecker(cfg)
	c.SendAlert("web1", "cpu", "critical", "[위험] web1 CPU", "")
	c.setDown("web1", true)
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 0 || len(c.unacked) != 1 {
		t.Fatalf("down host cpu escalated: %+v", got)
	}
	// 끊긴 시간만큼 확인 대기 시간이 멈춤 — 끊기기 1분 전 생긴 대기는 복귀 뒤 9분 더 기다림
	since := time.Now().Add(-time.Hour)
	c.escMu.Lock()
	c.downHosts["web1"] = since
	for _, p := range c.unacked {
		p.at = since.Add(-time.Minute)
	}
	c.escMu.Unlock()
	c.SendAlert("web1", "memory", "critical", "[위험] web1 메모리", "") // 복귀 주기에 새로 생긴 대기는 미루지 않음
	c.setDown("web1", false)
	drain(c.notifier)
	back := time.Now()
	c.escalate(cfg, back.Add(5*time.Minute))
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("right after return: %+v", got)
	}
	c.escalate(cfg, back.Add(10*time.Minute+time.Second))
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "2건") {
		t.Fatalf("after remaining time: %+v", got)
	}

	// 같은 주기에 여러 건이면 한 통
	c, _ = newTestChecker(cfg)
	for _, h := range []string{"a", "b", "c"} {
		c.SendAlert(h, "snmp", "critical", "[위험] "+h, "")
	}
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "3건") {
		t.Fatalf("grouped escalation: %+v", got)
	}

	// 문자 사용을 끄면 2차 담당자 문자도 안 나감
	cfg.Channels.SMS = SMSGateway{Enabled: false, URL: "http://sms"}
	cfg.Channels.Escalation.Phones = []string{"01011112222"}
	c, _ = newTestChecker(cfg)
	c.SendAlert("web1", "down", "critical", "[위험] web1", "")
	drain(c.notifier)
	c.escalate(cfg, now.Add(11*time.Minute))
	if got := drain(c.notifier); len(got) != 1 || len(got[0].Phones) != 0 {
		t.Fatalf("sms disabled escalation: %+v", got)
	}
}

func TestAlertsDisabled(t *testing.T) {
	cfg := escCfg(10)
	cfg.Enabled = false
	c, _ := newTestChecker(cfg)
	h := &memHistory{}
	c.history = h
	c.SendAlert("sw1", "snmp", "critical", "[위험] sw1", "")
	if got := drain(c.notifier); len(got) != 0 || len(h.subjects) != 1 {
		t.Fatalf("disabled: sent=%d history=%v", len(got), h.subjects)
	}
	if len(c.unacked) != 0 {
		t.Fatal("disabled tracked")
	}
}

func TestSendGroup(t *testing.T) {
	cfg := escCfg(10)
	c, _ := newTestChecker(cfg)
	h := &memHistory{}
	c.history = h
	c.Ack("sw2", "snmp", "critical", "kim")
	c.SendGroup("critical", "[위험] 네트워크 장비 %d대 응답 없음", []Alert{
		{Host: "sw1", Category: "snmp", Subject: "[위험] sw1 응답 없음"},
		{Host: "sw2", Category: "snmp", Subject: "[위험] sw2 응답 없음"},
		{Host: "sw3", Category: "snmp", Subject: "[위험] sw3 응답 없음"},
		{Host: "sw4", Category: "snmp", Subject: "[위험] sw4 응답 없음"},
	})
	got := drain(c.notifier)
	if len(got) != 1 || got[0].Subject != "[위험] 네트워크 장비 3대 응답 없음" || strings.Contains(got[0].Body, "sw2") || len(h.subjects) != 4 {
		t.Fatalf("group: %+v history=%v", got, h.subjects)
	}
	if len(c.unacked) != 3 {
		t.Fatalf("group tracked: %d", len(c.unacked))
	}
	// 적으면 하나씩
	c.SendGroup("info", "[복구] %d대", []Alert{{Host: "sw1", Category: "snmp", Subject: "[복구] sw1"}})
	if got := drain(c.notifier); len(got) != 1 || got[0].Subject != "[복구] sw1" {
		t.Fatalf("small group: %+v", got)
	}
}

func TestLaneOrderAndDrop(t *testing.T) {
	l := &lane{name: "t", limit: 3, wake: make(chan struct{}, 1)}
	l.push(Notice{Severity: "critical", Subject: "c1"})
	l.push(Notice{Severity: "warning", Subject: "w1"})
	l.push(Notice{Severity: "info", Subject: "r1"})
	l.push(Notice{Severity: "warning", Subject: "w2"})  // 가득 차면 주의는 버림
	l.push(Notice{Severity: "critical", Subject: "c2"}) // 위험은 가장 오래된 주의를 밀어냄
	var got []string
	for {
		m, ok := l.pop()
		if !ok {
			break
		}
		got = append(got, m.Subject)
	}
	// 넣은 순서 그대로 — 위험·복구가 뒤바뀌지 않음
	if strings.Join(got, ",") != "c1,r1,c2" {
		t.Fatalf("order: %v", got)
	}
}

func TestRedactErr(t *testing.T) {
	n := NewNotifier(nil)
	err := n.SendSMS(SMSGateway{URL: "http://127.0.0.1:1/send?key=SECRET&to={{phone}}", Method: "GET"}, "01011112222", "x")
	if err == nil {
		t.Fatal("want error")
	}
	if r := RedactErr(err); strings.Contains(r, "SECRET") || strings.Contains(r, "01011112222") {
		t.Errorf("leak: %s", r)
	}
	if r := RedactErr(&StatusError{Code: 401, Body: "key SECRET invalid"}); r != "HTTP 401" {
		t.Errorf("status: %s", r)
	}
}

func TestTrip(t *testing.T) {
	n := NewNotifier(nil)
	m := Notice{Severity: "critical", Subject: "x"}
	for i := 0; i < tripAfter; i++ {
		n.record("k", "t", io.EOF)
	}
	if n.allow("k", "t", m) {
		t.Fatal("not tripped")
	}
	n.trips["k"].until = time.Now().Add(-time.Second) // 차단 시간이 지나면 한 건만 시험 발송
	if !n.allow("k", "t", m) || n.allow("k", "t", m) {
		t.Fatal("half-open probe")
	}
	// 복구·2차 통보는 차단 중에도 보냄
	if !n.allow("k", "t", Notice{Severity: "info"}) || !n.allow("k", "t", Notice{Severity: "critical", Escalate: true}) {
		t.Fatal("one-shot blocked")
	}
	n.record("k", "t", nil)
	if !n.allow("k", "t", m) {
		t.Fatal("not reset")
	}
}

func TestSMSGetHasNoBody(t *testing.T) {
	var cl int64 = -2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cl = r.ContentLength }))
	defer srv.Close()
	g := SMSGateway{URL: srv.URL + "?to={{phone}}", Method: "GET", Body: `{"to":"{{phone}}"}`}
	if err := NewNotifier(nil).SendSMS(g, "01011112222", "x"); err != nil {
		t.Fatal(err)
	}
	if cl != 0 {
		t.Errorf("GET body length %d", cl)
	}
}

func TestWebhookLanesPerTarget(t *testing.T) {
	n := NewNotifier(nil)
	n.Enqueue(Notice{Severity: "critical", Subject: "x", Webhooks: []Webhook{{Name: "a", URL: "http://a"}, {Name: "b", URL: "http://b"}}})
	for _, u := range []string{"http://a", "http://b"} {
		m, ok := n.hookLane(u).pop()
		if !ok || len(m.Webhooks) != 1 || m.Webhooks[0].URL != u {
			t.Fatalf("lane %s: %+v", u, m)
		}
	}
}

// 로그 룰 알림(에이전트 입력)이 넘쳐도 먼저 대기 중이던 실제 위험 알림은 남아야 함
func TestLaneDropsLowBeforeCritical(t *testing.T) {
	l := &lane{name: "t", limit: 3, wake: make(chan struct{}, 1)}
	l.push(Notice{Severity: "critical", Subject: "real"})
	for i := 0; i < 100; i++ {
		l.push(Notice{Severity: "critical", Subject: "rule", Low: true})
	}
	l.push(Notice{Severity: "critical", Subject: "real2"})
	var got []string
	for {
		m, ok := l.pop()
		if !ok {
			break
		}
		got = append(got, m.Subject)
	}
	if strings.Join(got, ",") != "real,rule,real2" {
		t.Fatalf("got %v", got)
	}
}
