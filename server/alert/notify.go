package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 웹훅·문자 서버가 멈춰도 다음 알림이 밀리지 않게 짧게 끊음
const channelTimeout = 10 * time.Second

var sevRank = map[string]int{"info": 0, "warning": 1, "critical": 2}

// min이 비었으면 def 기준
func severityAtLeast(sev, min, def string) bool {
	if min == "" {
		min = def
	}
	return sevRank[sev] >= sevRank[min]
}

// Notice는 한 번에 보낼 알림 — 받을 곳은 넣을 때의 설정으로 정해 둠 (보내는 사이 설정이 바뀌어도 섞이지 않게)
type Notice struct {
	Severity string
	Subject  string
	Body     string
	Emails   []string
	Phones   []string
	Webhooks []Webhook
	SMS      SMSGateway
	Escalate bool // 2차 통보 — 다시 보내는 경로가 없는 한 번짜리
	Low      bool // 에이전트가 보낸 내용으로 만든 알림(로그 룰) — 대기열이 넘치면 실제 장애 알림보다 먼저 버림
}

// 경로마다 대기열과 작업자를 따로 둠 — 메일 서버가 막혀도 문자·웹훅은 나가야 함 (대체 경로가 같이 멈추면 의미가 없음).
// 꺼내는 순서는 넣은 순서 그대로 — 위험·복구가 뒤바뀌면 마지막으로 받은 알림이 실제 상태와 달라짐
type lane struct {
	name  string
	limit int
	mu    sync.Mutex
	q     []Notice
	wake  chan struct{}
	send  func(Notice)
}

// Notifier는 메일·웹훅·문자를 감지 루프 밖에서 보냄 — 예전엔 메일을 보내는 동안(최대 30초×건수) 다른 감지가 모두 멈췄음
type Notifier struct {
	email  *EmailConfig
	client *http.Client
	mail   *lane
	sms    *lane
	hookMu sync.Mutex
	hooks  map[string]*lane // 웹훅 주소별 — 응답 없는 대상이 자기 대기열만 막게
	ctx    context.Context  // Start 뒤에 생기는 웹훅 대기열의 작업자용
	tripMu sync.Mutex
	trips  map[string]*trip
}

func NewNotifier(email *EmailConfig) *Notifier {
	n := &Notifier{email: email, client: &http.Client{Timeout: channelTimeout}, trips: map[string]*trip{}, hooks: map[string]*lane{}}
	n.mail = newLane("메일", n.deliverMail)
	n.sms = newLane("문자", n.deliverSMS)
	return n
}

func newLane(name string, send func(Notice)) *lane {
	return &lane{name: name, limit: 2000, wake: make(chan struct{}, 1), send: send}
}

func (n *Notifier) Start(ctx context.Context) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	n.ctx = ctx
	go n.mail.run(ctx)
	go n.sms.run(ctx)
	for _, l := range n.hooks {
		go l.run(ctx)
	}
}

func (n *Notifier) hookLane(url string) *lane {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	l := n.hooks[url]
	if l == nil {
		l = newLane("웹훅", n.deliverHooks)
		n.hooks[url] = l
		if n.ctx != nil {
			go l.run(n.ctx)
		}
	}
	return l
}

func (l *lane) run(ctx context.Context) {
	for {
		if m, ok := l.pop(); ok {
			l.send(m)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
		}
	}
}

func (l *lane) pop() (Notice, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.q) == 0 {
		return Notice{}, false
	}
	m := l.q[0]
	l.q = l.q[1:]
	return m, true
}

// push는 가득 차도 감지 루프를 막지 않음. 넘치면 주의·로그 룰 알림부터 버림 — 없으면 가장 오래된 것을 버리고 지금 것을 넣음
func (l *lane) push(m Notice) {
	l.mu.Lock()
	if len(l.q) >= l.limit {
		drop := -1
		if m.expendable() {
			l.mu.Unlock()
			log.Printf("%s 대기열이 가득 차 알림 생략: %s", l.name, m.Subject)
			return
		}
		for i, x := range l.q {
			if x.expendable() {
				drop = i
				break
			}
		}
		if drop < 0 {
			drop = 0
		}
		log.Printf("%s 대기열이 가득 차 알림 생략: %s", l.name, l.q[drop].Subject)
		l.q = append(l.q[:drop], l.q[drop+1:]...)
	}
	l.q = append(l.q, m)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (m Notice) expendable() bool { return m.Severity == "warning" || m.Low }

// Enqueue는 경로별 대기열에 나눠 넣음 (알림 이력은 이미 저장됨)
func (n *Notifier) Enqueue(m Notice) {
	if n.email != nil && len(m.Emails) > 0 {
		n.mail.push(m)
	}
	for _, w := range m.Webhooks {
		one := m
		one.Webhooks = []Webhook{w}
		n.hookLane(w.URL).push(one)
	}
	if len(m.Phones) > 0 {
		n.sms.push(m)
	}
}

func (n *Notifier) deliverMail(m Notice) {
	// 받는 사람 묶음별로 셈 — 1차 수신자 쪽 오류로 2차 담당자 메일까지 막히지 않게
	key := "mail " + strings.Join(m.Emails, ",")
	if !n.allow(key, "메일", m) {
		return
	}
	err := n.email.SendTo(m.Emails, m.Subject, m.Body)
	n.record(key, "알림 메일", err)
	if err == nil {
		log.Printf("알림 메일 발송: %s", m.Subject)
	}
}

func (n *Notifier) deliverHooks(m Notice) {
	for _, w := range m.Webhooks {
		key := "webhook " + w.URL
		if n.allow(key, "웹훅 "+w.Name, m) {
			n.record(key, "웹훅 "+w.Name, n.SendWebhook(w, m))
		}
	}
}

// 실패는 알림 한 건 단위로 셈 — 번호가 여럿이면 한 건으로 바로 차단되던 문제
func (n *Notifier) deliverSMS(m Notice) {
	key := "sms " + m.SMS.URL
	if !n.allow(key, "문자 게이트웨이", m) {
		return
	}
	sent := 0
	var last error
	for i, p := range m.Phones {
		err := n.SendSMS(m.SMS, p, smsText(m))
		if err == nil {
			sent++
			continue
		}
		last = err
		log.Printf("문자(%s) 발송 실패: %s", maskPhone(p), RedactErr(err))
		// 접속 자체가 안 되면 게이트웨이 문제 — 남은 번호마다 10초씩 기다리지 않음 (HTTP 오류는 번호 문제일 수 있어 계속)
		var ue *url.Error
		if sent == 0 && errors.As(err, &ue) {
			if rest := len(m.Phones) - i - 1; rest > 0 {
				log.Printf("문자 게이트웨이 접속 실패 — 남은 %d개 번호 생략: %s", rest, m.Subject)
			}
			break
		}
	}
	if sent > 0 {
		last = nil
	}
	n.record(key, "문자 게이트웨이", last)
}

// 연달아 실패한 대상은 잠시 건너뜀 — 닿지 않는 주소가 건마다 타임아웃을 기다려 뒤 알림이 밀리지 않게.
// 차단 중엔 tripFor마다 한 건만 시험 삼아 보내고(성공하면 풀림) 나머지는 기록만 남기고 건너뜀.
// 복구와 2차 통보는 다시 보내는 경로가 없어(위험은 쿨다운 뒤 재발송됨) 건너뛰면 받는 쪽 마지막 상태가 틀린 채 남음 — 차단 중에도 보냄.
// 대상마다 대기열이 따로라 기다림은 그 대상(어차피 응답 없는 곳)에만 걸림
const (
	tripAfter = 3
	tripFor   = 2 * time.Minute
)

type trip struct {
	fails int
	until time.Time
}

func (n *Notifier) allow(key, label string, m Notice) bool {
	n.tripMu.Lock()
	defer n.tripMu.Unlock()
	t := n.trips[key]
	if t == nil || t.fails < tripAfter || m.Severity == "info" || m.Escalate {
		return true
	}
	if now := time.Now(); !now.Before(t.until) {
		t.until = now.Add(tripFor) // 이번 건이 시험 발송 — 끝나기 전에 들어온 건은 건너뜀
		return true
	}
	log.Printf("%s 차단 중 — 발송 생략: %s", label, m.Subject)
	return false
}

func (n *Notifier) record(key, label string, err error) {
	n.tripMu.Lock()
	defer n.tripMu.Unlock()
	if err == nil {
		delete(n.trips, key)
		return
	}
	t := n.trips[key]
	if t == nil {
		t = &trip{}
		n.trips[key] = t
	}
	t.fails++
	log.Printf("%s 발송 실패 (%d회 연속): %s", label, t.fails, RedactErr(err))
	if t.fails == tripAfter {
		t.until = time.Now().Add(tripFor)
		log.Printf("%s %d회 연속 실패 — %v마다 한 건씩만 시험 발송", label, t.fails, tripFor)
	}
}

// RedactErr는 로그·화면에 남길 오류 문구 — 요청 주소의 경로·쿼리(키·전화번호가 들어감)와 응답 본문을 뺌
func RedactErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		host := "?"
		if u, perr := url.Parse(ue.URL); perr == nil {
			host = u.Scheme + "://" + u.Host
		}
		return fmt.Sprintf("%s %s: %v", ue.Op, host, ue.Err)
	}
	var se *StatusError
	if errors.As(err, &se) {
		return fmt.Sprintf("HTTP %d", se.Code)
	}
	return err.Error()
}

// 문자는 길이 제한이 있어 제목만 — 본문 첫 줄(대상)과 위치는 제목에 이미 들어 있음
func smsText(m Notice) string { return "[OWLmon] " + m.Subject }

func maskPhone(p string) string {
	if len(p) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(p)-4) + p[len(p)-4:]
}

func webhookPayload(format string, m Notice) ([]byte, error) {
	if format == "slack" {
		return json.Marshal(map[string]string{"text": "*[OWLmon] " + m.Subject + "*\n" + m.Body})
	}
	return json.Marshal(map[string]string{"source": "owlmon", "severity": m.Severity, "subject": m.Subject, "body": m.Body, "sent_at": time.Now().Format(time.RFC3339)})
}

func (n *Notifier) SendWebhook(w Webhook, m Notice) error {
	payload, err := webhookPayload(w.Format, m)
	if err != nil {
		return err
	}
	resp, err := n.client.Post(w.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	return checkStatus(resp)
}

// renderBody는 자리표시자를 본문 형식에 맞게 이스케이프해 넣음 — 메시지에 따옴표·&가 있어도 요청이 깨지지 않게
func renderBody(tmpl, contentType string, vars map[string]string) string {
	esc := func(v string) string { return v }
	switch {
	case strings.Contains(contentType, "json"):
		esc = func(v string) string {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false) // 문자 내용의 & < > 를 \u0026 처럼 바꾸면 게이트웨이가 그대로 찍는 경우가 있음
			_ = enc.Encode(v)
			b := bytes.TrimSpace(buf.Bytes())
			return string(b[1 : len(b)-1])
		}
	case strings.Contains(contentType, "x-www-form-urlencoded"):
		esc = url.QueryEscape
	}
	for k, v := range vars {
		tmpl = strings.ReplaceAll(tmpl, "{{"+k+"}}", esc(v))
	}
	return tmpl
}

func (n *Notifier) SendSMS(g SMSGateway, phone, text string) error {
	if g.URL == "" {
		return fmt.Errorf("문자 게이트웨이 주소가 없습니다")
	}
	method := g.Method
	if method == "" {
		method = http.MethodPost
	}
	ct := g.ContentType
	if ct == "" {
		ct = "application/json"
	}
	vars := map[string]string{"phone": phone, "message": text}
	body := renderBody(g.Body, ct, vars)
	target := g.URL
	if method == http.MethodGet {
		target = renderBody(g.URL, "application/x-www-form-urlencoded", vars)
	}
	var rdr io.Reader
	if method != http.MethodGet {
		rdr = strings.NewReader(body) // 화면이 GET에선 본문 칸을 숨기므로 남은 양식을 보내면 안 됨
	}
	req, err := http.NewRequest(method, target, rdr)
	if err != nil {
		return err
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", ct)
	}
	for k, v := range g.Headers {
		req.Header.Set(k, v)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	return checkStatus(resp)
}

// StatusError의 본문은 시험 발송 화면에서만 보여 줌 — 로그에는 RedactErr로 상태 코드만
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Code, e.Body) }

func checkStatus(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	return &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
}

// routeNotice는 설정에서 이 알림을 받을 곳을 고름 — 복구 알림은 각 경로의 '복구도 보냄' 설정을 따름
func routeNotice(cfg AlertConfig, defaultTo []string, severity, subject, body string) Notice {
	recovery := strings.HasPrefix(subject, "[복구]")
	m := Notice{Severity: severity, Subject: subject, Body: body, SMS: cfg.Channels.SMS}
	m.Emails = defaultTo
	if len(cfg.Recipients) > 0 {
		m.Emails = cfg.Recipients
	}
	for _, w := range cfg.Channels.Webhooks {
		if w.URL != "" && (severityAtLeast(severity, w.MinSeverity, "warning") || (recovery && w.Recovery)) {
			m.Webhooks = append(m.Webhooks, w)
		}
	}
	g := cfg.Channels.SMS
	if g.Enabled && g.URL != "" && (severityAtLeast(severity, g.MinSeverity, "critical") || (recovery && g.Recovery)) {
		m.Phones = g.Phones
	}
	return m
}
