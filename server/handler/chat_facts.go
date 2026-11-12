package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/anomaly"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/dpm"
	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/loginsight"
	"github.com/seongJae/owlmon/server/prom"
	"github.com/seongJae/owlmon/server/ssl"
	"github.com/seongJae/owlmon/server/synthetic"
)

const (
	routeOverview = "overview"
	routeHost     = "host"
	routeUsage    = "usage"
	routeDisk     = "disk_forecast"
	routeSSL      = "ssl"
	routeHistory  = "alert_history"
	routeInsights = "insights"
	routeAction   = "action"
	routeNone     = "none"

	chatListLimit       = 8
	chatActionReply     = "OWL AI는 조회와 설명만 할 수 있어 직접 실행하지 못합니다. 작업이 필요하면 설치 업체나 담당자에게 요청해 주세요."
	chatRouterFailReply = "AI 연결에 실패했습니다. 아래 버튼을 누르거나 \"지금 서버 상태\"라고 입력하면 AI 없이 조회합니다."
	chatGeneralNote     = "(일반적인 설명입니다. 이 기관 서버의 실제 상태를 확인한 답변이 아닙니다.)"
	chatFetchFailed     = "상태를 조회하지 못했습니다. 문제가 없다는 뜻이 아닙니다. 잠시 후 다시 시도하거나 '지금 상태' 메뉴를 확인해 주세요."
)

var validRoutes = map[string]bool{
	routeOverview: true, routeHost: true, routeUsage: true, routeDisk: true, routeSSL: true,
	routeHistory: true, routeInsights: true, routeAction: true, routeNone: true,
}

// 화면 버튼과 글자까지 같아야 한다 — AI가 멈춰도 이 답변은 나가도록 분류를 거치지 않는다
var chipRoutes = map[string]string{
	"지금 서버 상태":    routeOverview,
	"디스크 언제 차?":   routeDisk,
	"인증서 만료 확인":   routeSSL,
	"어제 알림 내역":    routeHistory,
	"AI 로그 분석 결과": routeInsights,
}

var chatKST = time.FixedZone("KST", 9*3600)

// 데이터 답변을 일반 대화 맥락에서 빼는 표식이다. 모든 데이터 답변은 chatHeader로 시작한다
var reChatDataAnswer = regexp.MustCompile(`^\d{1,2}월 \d{1,2}일 \d{2}:\d{2} 기준`)

func chatHeader(now time.Time) string {
	return fmt.Sprintf("%d월 %d일 %02d:%02d 기준", now.Month(), now.Day(), now.Hour(), now.Minute())
}

// 대화 라벨을 넣어 다른 턴처럼 보이게 만드는 것을 막는다
var turnLabels = strings.NewReplacer("[사용자]", "(사용자)", "[도우미]", "(도우미)", "[질문]", "(질문)", "[직전 질문]", "(직전 질문)")

func cleanTurn(s string, maxRunes int) string {
	s = turnLabels.Replace(strings.TrimSpace(s))
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "...(생략)"
	}
	return s
}

func classifyRoute(ctx context.Context, p llm.Provider, prev, q string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	user := "[질문] " + cleanTurn(q, 500)
	if prev != "" {
		user = "[직전 질문] " + cleanTurn(prev, 500) + "\n" + user
	}
	text, err := p.Complete(ctx, llm.SystemPromptChatRouter, user)
	if err != nil {
		return "", err
	}
	var out struct {
		Route string `json:"route"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return "", fmt.Errorf("분류 응답 파싱 실패: %w", err)
	}
	if !validRoutes[out.Route] {
		return "", fmt.Errorf("목록 밖 분류: %q", out.Route)
	}
	return out.Route, nil
}

// viewer도 GET으로 볼 수 있는 데이터만 둔다. 챗봇이 조회 권한을 넓히지 않게 하기 위함이다
type ChatDeps struct {
	Status           *StatusHandler
	Checker          *alert.Checker
	History          *db.AlertHistoryStore
	Insights         *loginsight.Store
	Health           func() loginsight.Health
	SNMP             *db.SNMPDeviceStore
	Synthetic        *db.SyntheticStore
	SyntheticChecker *synthetic.Checker
	DPM              *db.DPMStore
	DPMPoller        *dpm.Poller
}

// 호스트·마운트는 인증 없는 수집 경로에서 오는 값이라 줄을 위조하거나 서식을 깨지 못하게 한다
func chatLabel(s string) string { return chatCut(s, 64) }

func chatCut(s string, maxRunes int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r):
			return ' '
		case r == '*' || r == '`':
			return -1
		}
		return r
	}, s)
	r := []rune(strings.TrimSpace(s))
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return string(r)
}

func capLines(items []string) []string {
	if len(items) <= chatListLimit {
		return items
	}
	out := append([]string{}, items[:chatListLimit]...)
	return append(out, fmt.Sprintf("- 외 %d개 항목", len(items)-chatListLimit))
}

// 안내문은 판정 바로 다음에 둔다. 첫 줄은 데이터 답변 표식이라 앞에 넣을 수 없다
func withNote(answer, note string) string {
	if note == "" {
		return answer
	}
	first, rest, ok := strings.Cut(answer, "\n\n")
	if !ok {
		return joinChat(answer, note)
	}
	return joinChat(first, note, rest)
}

func joinChat(blocks ...string) string {
	out := blocks[:0:0]
	for _, b := range blocks {
		if b != "" {
			out = append(out, b)
		}
	}
	return strings.Join(out, "\n\n")
}

var reChatSafeHost = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func chatFindHost(q string, hosts []string) string {
	q = strings.ToLower(q)
	found := ""
	for _, h := range hosts {
		if !reChatSafeHost.MatchString(h) || len(h) <= len(found) {
			continue
		}
		re := regexp.MustCompile(`(?:^|[^a-z0-9._-])` + regexp.QuoteMeta(strings.ToLower(h)) + `(?:$|[^a-z0-9._-]|\.(?:$|[^a-z0-9]))`)
		if re.MatchString(q) {
			found = h
		}
	}
	return found
}

type chatPeriod struct {
	from, to time.Time
	label    string
}

var (
	reChatUnsupported = regexp.MustCompile(`재작년|작년|지지난|분기|상반기|하반기|올해|연초`)
	reChatDate        = regexp.MustCompile(`(\d{1,2})월(\d{1,2})일`)
	reChatRange       = regexp.MustCompile(`(?:최근|지난)(\d+)(시간|일|주|개월|달)|(\d+)(시간|일|주|개월|달)(?:간|동안|사이)`)
	reChatDaysAgo     = regexp.MustCompile(`(\d+)일전`)
	chatPeriodWords   = strings.NewReplacer("일주일", "1주", "주일", "주", "한달", "1개월", "이틀", "2일", "사흘", "3일")
)

func chatNorm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }

// 기간은 AI에게 맡기지 않는다 — 측정에서 "어제 밤"을 24시간으로 잘못 읽었다
func chatParsePeriod(q string, now time.Time) (p chatPeriod, found, unsupported bool) {
	n := chatPeriodWords.Replace(chatNorm(q))
	if reChatUnsupported.MatchString(n) {
		return chatPeriod{}, false, true
	}
	now = now.In(chatKST)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, chatKST)
	monday := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	month := today.AddDate(0, 0, 1-today.Day())
	has := func(ws ...string) bool {
		for _, w := range ws {
			if strings.Contains(n, w) {
				return true
			}
		}
		return false
	}
	clip := func(t time.Time) time.Time {
		if t.After(now) {
			return now
		}
		return t
	}
	switch {
	case reChatDate.MatchString(n):
		m := reChatDate.FindStringSubmatch(n)
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		t := time.Date(now.Year(), time.Month(mo), d, 0, 0, 0, 0, chatKST)
		if t.After(now) {
			t = t.AddDate(-1, 0, 0)
		}
		if t.Month() != time.Month(mo) || t.Day() != d {
			return chatPeriod{}, false, true
		}
		return chatPeriod{t, t.AddDate(0, 0, 1), fmt.Sprintf("%d월 %d일", mo, d)}, true, false
	// 짧은 낱말은 공백을 지운 문장에서 "지금 주요"처럼 단어를 넘어 걸리므로 원문에서 찾는다
	case has("그저께") || strings.Contains(q, "그제"):
		return chatPeriod{today.AddDate(0, 0, -2), today.AddDate(0, 0, -1), "그저께"}, true, false
	case has("어제밤", "어젯밤", "간밤", "지난밤"):
		return chatPeriod{today.Add(-6 * time.Hour), clip(today.Add(6 * time.Hour)), "어젯밤(18시~6시)"}, true, false
	case has("어제", "어저께"):
		return chatPeriod{today.AddDate(0, 0, -1), today, "어제"}, true, false
	case has("오늘새벽"):
		return chatPeriod{today, clip(today.Add(6 * time.Hour)), "오늘 새벽(0시~6시)"}, true, false
	case has("오늘") || strings.Contains(q, "금일"):
		return chatPeriod{today, now, "오늘"}, true, false
	case has("지난주", "저번주"):
		return chatPeriod{monday.AddDate(0, 0, -7), monday, "지난주(월~일)"}, true, false
	case has("이번주") || strings.Contains(q, "금주"):
		return chatPeriod{monday, now, "이번 주"}, true, false
	case has("지난달", "저번달"):
		return chatPeriod{month.AddDate(0, -1, 0), month, "지난달"}, true, false
	case has("이번달") || strings.Contains(q, "금월"):
		return chatPeriod{month, now, "이번 달"}, true, false
	}
	if m := reChatRange.FindStringSubmatch(n); m != nil {
		s, unit := m[1], m[2]
		if s == "" {
			s, unit = m[3], m[4]
		}
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 || v > 366 {
			return chatPeriod{}, false, true
		}
		var from time.Time
		switch unit {
		case "시간":
			from = now.Add(-time.Duration(v) * time.Hour)
		case "일":
			from = now.AddDate(0, 0, -v)
		case "주":
			from = now.AddDate(0, 0, -7*v)
		default:
			from = now.AddDate(0, -v, 0)
		}
		return chatPeriod{from, now, "최근 " + s + unit}, true, false
	}
	if m := reChatDaysAgo.FindStringSubmatch(n); m != nil {
		v, err := strconv.Atoi(m[1])
		if err != nil || v > 366 {
			return chatPeriod{}, false, true
		}
		t := today.AddDate(0, 0, -v)
		return chatPeriod{t, t.AddDate(0, 0, 1), m[1] + "일 전"}, true, false
	}
	return chatPeriod{}, false, false
}

func (d *ChatDeps) answer(ctx context.Context, route, q, prev string, now time.Time) string {
	now = now.In(chatKST)
	head := chatHeader(now)
	snap := d.Status.snapshot()
	failed := head + " · " + chatFetchFailed

	// 버튼은 전체 조회다. 직전 대화의 서버로 좁히면 다른 서버의 위험이 답변에서 사라진다
	host := ""
	if _, isChip := chipRoutes[strings.TrimSpace(q)]; !isChip {
		host = chatFindHost(q, snap.hosts())
		if host == "" && route == routeHost {
			host = chatFindHost(prev, snap.hosts())
		}
	}

	note := ""
	switch route {
	case routeOverview, routeHost, routeUsage:
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, chatKST)
		if p, ok, unsupported := chatParsePeriod(q, now); unsupported || (ok && (p.from.Before(today) || p.to.Before(now))) {
			note = "과거 시점의 수치는 조회할 수 없어 지금 기준으로 답합니다."
		}
	case routeHistory, routeInsights:
		if snap.Failed && len(snap.Seen) == 0 {
			note = "서버 목록을 조회하지 못해 서버를 구분하지 않고 전체 기준으로 답합니다."
		}
	}

	switch route {
	case routeHost, routeOverview, routeUsage, routeDisk:
		if snap.Failed {
			return failed
		}
	}

	switch route {
	case routeHost:
		if host != "" {
			return withNote(renderChatHost(head, host, snap, now), note)
		}
		fallthrough
	case routeOverview:
		others, err := d.otherProblems(ctx, snap)
		if err != nil {
			log.Printf("[chat] 장비 상태 조회 실패: %v", err)
			return failed
		}
		high, err := d.highInsights(ctx, now)
		if err != nil {
			log.Printf("[chat] 로그 분석 조회 실패: %v", err)
			return failed
		}
		if route == routeHost {
			note = joinChat("그 이름의 서버를 찾지 못해 전체 현황을 보여드립니다.", note)
		}
		return withNote(renderChatOverview(head, snap, others, high, now), note)
	case routeUsage:
		return withNote(renderChatUsage(head, q, snap), note)
	case routeDisk:
		return renderChatDisk(head, d.Checker.Predictor.GetPredictions(), snap, host)
	case routeSSL:
		return renderChatSSL(head, d.Checker.SSLChecker.GetResults(), now)
	case routeHistory:
		return withNote(d.answerHistory(ctx, head, q, host, now), note)
	case routeInsights:
		return withNote(d.answerInsights(ctx, head, host, now), note)
	}
	return failed
}

type chatProblem struct {
	label, unit string
	total       int
	bad         []string
}

func (d *ChatDeps) otherProblems(ctx context.Context, snap fleetSnapshot) ([]chatProblem, error) {
	var out []chatProblem
	if d.SNMP != nil {
		devices, err := d.SNMP.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(devices) > 0 {
			rs, err := prom.Query(d.Status.prometheusURL, "snmp_device_up")
			if err != nil {
				return nil, err
			}
			up := map[string]bool{}
			for _, r := range rs {
				up[r.Metric["device"]] = r.Value > 0
			}
			g := chatProblem{label: "네트워크 장비", unit: "대", total: len(devices)}
			for _, dev := range devices {
				if !up[dev.Name] {
					g.bad = append(g.bad, chatLabel(dev.Name))
				}
			}
			out = append(out, g)
		}
	}

	web := chatProblem{label: "웹사이트", unit: "곳"}
	if d.Synthetic != nil && d.SyntheticChecker != nil {
		monitors, err := d.Synthetic.List(ctx)
		if err != nil {
			return nil, err
		}
		latest := d.SyntheticChecker.GetLatest()
		for _, m := range monitors {
			if !m.Enabled {
				continue
			}
			web.total++
			if r, ok := latest[m.ID]; ok && !r.Success {
				web.bad = append(web.bad, chatLabel(m.Name))
			}
		}
	}
	if d.Checker != nil && d.Checker.SSLChecker != nil {
		for _, c := range d.Checker.SSLChecker.GetResults() {
			web.total++
			if c.Status == "expired" {
				web.bad = append(web.bad, chatLabel(c.Domain)+" 인증서 만료")
			}
		}
	}
	if web.total > 0 {
		out = append(out, web)
	}

	if d.DPM != nil && d.DPMPoller != nil {
		instances, err := d.DPM.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(instances) > 0 {
			latest := d.DPMPoller.GetLatest()
			g := chatProblem{label: "데이터베이스", unit: "개", total: len(instances)}
			for _, inst := range instances {
				if r, ok := latest[inst.ID]; ok && r.Metrics.Error != "" {
					g.bad = append(g.bad, chatLabel(inst.Name))
				}
			}
			out = append(out, g)
		}
	}
	return out, nil
}

func (d *ChatDeps) recentInsights(ctx context.Context, now time.Time) ([]loginsight.Insight, error) {
	return d.Insights.List(ctx, loginsight.Filter{From: now.Add(-24 * time.Hour), Limit: 200})
}

func (d *ChatDeps) highInsights(ctx context.Context, now time.Time) (int, error) {
	if d.Insights == nil || d.Health == nil {
		return 0, nil
	}
	items, err := d.recentInsights(ctx, now)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, g := range groupChatInsights(items, "") {
		if g.severity == loginsight.SeverityCritical || g.severity == loginsight.SeverityHigh {
			n++
		}
	}
	return n, nil
}

func renderChatAlertLine(a ActiveAlert, seen map[string]time.Time, now time.Time) string {
	msg := strings.NewReplacer("임계값", "기준", "(경고 ", "(기준 ").Replace(chatCut(a.Message, 200))
	if a.Category == "down" {
		msg = "서버와 연결이 끊김"
		if t, ok := seen[a.Host]; ok {
			msg += fmt.Sprintf(" (마지막 수신 %s 전)", chatAgo(now.Sub(t)))
		}
	}
	line := "- " + chatLabel(a.Host) + ": " + msg
	if a.Acked {
		line += " (확인함)"
	}
	return line
}

func chatAgo(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%d일", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%d시간", int(d.Hours()))
	}
	return fmt.Sprintf("%d분", int(d.Minutes()))
}

func sortChatAlerts(alerts []ActiveAlert) {
	rank := func(a ActiveAlert) int {
		r := 2
		switch {
		case a.Category == "down":
			r = 0
		case a.Severity == "critical":
			r = 1
		}
		if a.Acked {
			r += 3
		}
		return r
	}
	sort.SliceStable(alerts, func(i, j int) bool {
		if ri, rj := rank(alerts[i]), rank(alerts[j]); ri != rj {
			return ri < rj
		}
		if alerts[i].Value != alerts[j].Value {
			return alerts[i].Value > alerts[j].Value
		}
		return alerts[i].Host < alerts[j].Host
	})
}

func renderChatOverview(head string, snap fleetSnapshot, others []chatProblem, highInsights int, now time.Time) string {
	var active []ActiveAlert
	for _, a := range snap.Alerts {
		if !a.InMaintenance {
			active = append(active, a)
		}
	}
	sortChatAlerts(active)

	var critical, warning, acked []string
	crit, warn, ackN := 0, 0, 0
	for _, a := range active {
		line := renderChatAlertLine(a, snap.Seen, now)
		switch {
		case a.Acked:
			ackN++
			acked = append(acked, line)
		case a.Severity == "critical":
			crit++
			critical = append(critical, line)
		default:
			warn++
			warning = append(warning, line)
		}
	}

	badOthers := 0
	var otherLines []string
	var fine []string
	for _, g := range others {
		if len(g.bad) == 0 {
			fine = append(fine, g.label)
			continue
		}
		badOthers += len(g.bad)
		names := g.bad
		more := ""
		if len(names) > 3 {
			more = fmt.Sprintf(" 외 %d%s", len(names)-3, g.unit)
			names = names[:3]
		}
		otherLines = append(otherLines, fmt.Sprintf("- %s 문제 %d%s: %s%s", g.label, len(g.bad), g.unit, strings.Join(names, ", "), more))
	}

	hosts := snap.hosts()
	verdict := ""
	switch {
	case len(hosts) == 0:
		verdict = "등록된 서버가 없습니다"
	case len(snap.Down) == len(hosts):
		verdict = fmt.Sprintf("서버 %d대 모두 응답이 없습니다. 서버가 아니라 수집 경로 문제일 수도 있습니다", len(hosts))
	case crit+warn > 0:
		verdict = fmt.Sprintf("처리할 문제 %d건 (위험 %d · 주의 %d)", crit+warn, crit, warn)
	case badOthers > 0:
		verdict = "서버 경고는 없지만 다른 장비에 문제가 있습니다"
	default:
		verdict = "처리할 문제가 없습니다"
	}

	section := func(title string, lines []string) string {
		if len(lines) == 0 {
			return ""
		}
		return "**" + title + "**\n" + strings.Join(capLines(lines), "\n")
	}

	var extra []string
	if ackN > 0 {
		extra = append(extra, fmt.Sprintf("확인함 %d건", ackN))
	}
	if n := len(snap.Maint); n > 0 {
		extra = append(extra, fmt.Sprintf("점검 중 %d대 제외", n))
	}
	if len(fine) > 0 {
		extra = append(extra, strings.Join(fine, "·")+" 문제 없음")
	}

	insightLine := ""
	if highInsights > 0 {
		insightLine = fmt.Sprintf("AI 로그 분석(AI 추정)에서 위험 이상 %d건이 있습니다. 'AI 분석' 메뉴에서 확인해 주세요.", highInsights)
	}

	return joinChat(
		head+" · "+verdict,
		section("위험", critical),
		section("주의", warning),
		section("확인함", acked),
		section("다른 장비", otherLines),
		strings.Join(extra, " · "),
		insightLine,
		"자세히 보기: '지금 상태' 메뉴 (서비스 점검 결과는 포함하지 않습니다)",
	)
}

func renderChatHost(head, host string, snap fleetSnapshot, now time.Time) string {
	name := chatLabel(host)
	maint := ""
	for _, m := range snap.Maint {
		if m == host {
			maint = "점검 중입니다 (알림 꺼짐)."
		}
	}
	if snap.Down[host] {
		return joinChat(
			fmt.Sprintf("%s · %s: 응답 없음 (마지막 수신 %s 전)", head, name, chatAgo(now.Sub(snap.Seen[host]))),
			"현재 수치는 알 수 없습니다.",
			maint,
			"자세히 보기: '지금 상태' 메뉴",
		)
	}

	var mounts []string
	for m := range snap.Disk[host] {
		mounts = append(mounts, m)
	}
	sort.Slice(mounts, func(i, j int) bool { return snap.Disk[host][mounts[i]] > snap.Disk[host][mounts[j]] })
	var disks []string
	for _, m := range mounts {
		disks = append(disks, fmt.Sprintf("- %s: %.0f%%", chatLabel(m), snap.Disk[host][m]))
	}

	var alerts []ActiveAlert
	for _, a := range snap.Alerts {
		if a.Host == host {
			alerts = append(alerts, a)
		}
	}
	sortChatAlerts(alerts)
	alertBlock := "현재 경고 없음"
	if len(alerts) > 0 {
		var lines []string
		for _, a := range alerts {
			label := "주의"
			if a.Severity == "critical" {
				label = "위험"
			}
			lines = append(lines, strings.Replace(renderChatAlertLine(a, snap.Seen, now), "- "+name+":", "- "+label+":", 1))
		}
		alertBlock = "**경고**\n" + strings.Join(capLines(lines), "\n")
	}

	diskBlock := ""
	if len(disks) > 0 {
		diskBlock = "**디스크**\n" + strings.Join(capLines(disks), "\n")
	}
	return joinChat(
		fmt.Sprintf("%s · %s: CPU %.0f%% · 메모리 %.0f%%", head, name, snap.CPU[host], snap.Mem[host]),
		diskBlock,
		alertBlock,
		maint,
		"자세히 보기: '지금 상태' 메뉴",
	)
}

func renderChatUsage(head, q string, snap fleetSnapshot) string {
	n := chatNorm(q)
	wantCPU := strings.Contains(n, "cpu") || strings.Contains(n, "씨피유")
	wantMem := strings.Contains(n, "메모리") || strings.Contains(n, "램")
	wantDisk := strings.Contains(n, "디스크") || strings.Contains(n, "용량")
	all := !wantCPU && !wantMem && !wantDisk

	type row struct {
		label string
		value float64
	}
	block := func(title string, rows []row) string {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].value > rows[j].value })
		lines := make([]string, 0, len(rows))
		for _, r := range rows {
			lines = append(lines, fmt.Sprintf("- %s: %.0f%%", r.label, r.value))
		}
		return "**" + title + "**\n" + strings.Join(capLines(lines), "\n")
	}
	metric := func(values map[string]float64) []row {
		var rows []row
		for h, v := range values {
			if !snap.Down[h] {
				rows = append(rows, row{chatLabel(h), v})
			}
		}
		return rows
	}

	var up, down []string
	for _, h := range snap.hosts() {
		if snap.Down[h] {
			down = append(down, chatLabel(h))
		} else {
			up = append(up, h)
		}
	}
	first := fmt.Sprintf("%s · 응답 중인 서버 %d대 기준", head, len(up))
	if len(down) > 0 {
		names := down
		more := ""
		if len(names) > 3 {
			more = fmt.Sprintf(" 외 %d대", len(names)-3)
			names = names[:3]
		}
		first += " (제외: " + strings.Join(names, ", ") + more + ", 응답 없음)"
	}
	if len(up) == 0 {
		return first
	}

	cpu, mem, disk := "", "", ""
	if all || wantCPU {
		cpu = block("CPU 사용률", metric(snap.CPU))
	}
	if all || wantMem {
		mem = block("메모리 사용률", metric(snap.Mem))
	}
	if all || wantDisk {
		var rows []row
		for h, mounts := range snap.Disk {
			if snap.Down[h] {
				continue
			}
			best, bestV := "", -1.0
			for m, v := range mounts {
				if v > bestV {
					best, bestV = m, v
				}
			}
			rows = append(rows, row{chatLabel(h) + " " + chatLabel(best), bestV})
		}
		disk = block("디스크 사용률 (서버별 최대)", rows)
	}
	return joinChat(first, cpu, mem, disk, "자세히 보기: '사용량 추이' 메뉴")
}

func renderChatDisk(head string, preds []anomaly.DiskPrediction, snap fleetSnapshot, host string) string {
	footer := "최근 최대 6시간의 추세로 계산한 추정치입니다. 자세히 보기: '사용량 추이' 메뉴"
	scope := ""
	if host != "" {
		scope = chatLabel(host) + " "
	}
	var growing []anomaly.DiskPrediction
	flat, unstable, total, down := 0, 0, 0, 0
	for _, p := range preds {
		if host != "" && p.Host != host {
			continue
		}
		if snap.Down[p.Host] {
			down++
			continue
		}
		// 예측은 서버 재시작 전까지 메모리에 남는다. 더 이상 수집되지 않는 서버·마운트는 현재 값처럼 내보내지 않는다
		if _, live := snap.Disk[p.Host][p.Mountpoint]; !live {
			continue
		}
		total++
		switch {
		case p.DaysLeft < 0:
			flat++
		case p.R2 < 0.5:
			unstable++
		default:
			growing = append(growing, p)
		}
	}
	if total == 0 && down > 0 {
		return joinChat(head+" · "+scope+"응답이 없는 서버라 디스크 예측을 표시하지 않습니다.", "자세히 보기: '지금 상태' 메뉴")
	}
	if total == 0 {
		return joinChat(head+" · "+scope+"아직 디스크 예측 데이터가 없습니다. 서버 시작 후 1시간 정도 지나야 예측할 수 있습니다.", "자세히 보기: '사용량 추이' 메뉴")
	}
	sort.Slice(growing, func(i, j int) bool { return growing[i].DaysLeft < growing[j].DaysLeft })
	lines := make([]string, 0, len(growing))
	for _, p := range growing {
		if p.Current >= 95 {
			lines = append(lines, fmt.Sprintf("- %s %s: 현재 %.0f%%, 이미 95%% 이상입니다", chatLabel(p.Host), chatLabel(p.Mountpoint), p.Current))
			continue
		}
		when := fmt.Sprintf("약 %.0f일 후", p.DaysLeft)
		if p.DaysLeft < 1 {
			when = "하루 안에"
		}
		lines = append(lines, fmt.Sprintf("- %s %s: 현재 %.0f%%, 이 추세면 %s 95%% 도달", chatLabel(p.Host), chatLabel(p.Mountpoint), p.Current, when))
	}
	first := head + " · " + scope + "디스크가 늘어나는 추세가 뚜렷한 곳이 없습니다"
	if len(lines) > 0 {
		first = fmt.Sprintf("%s · %s디스크가 늘어나는 곳 %d개 (추정)", head, scope, len(lines))
	}
	var rest []string
	if flat > 0 {
		rest = append(rest, fmt.Sprintf("늘어나는 추세 없음 %d개", flat))
	}
	if unstable > 0 {
		rest = append(rest, fmt.Sprintf("추세가 불규칙해 예측 불가 %d개", unstable))
	}
	body := ""
	if len(lines) > 0 {
		body = strings.Join(capLines(lines), "\n")
	}
	return joinChat(first, body, strings.Join(rest, " · "), footer)
}

func renderChatSSL(head string, certs []ssl.CertInfo, now time.Time) string {
	footer := "6시간마다 점검합니다. 자세히 보기: '웹사이트' 메뉴"
	if len(certs) == 0 {
		return joinChat(head+" · 등록된 인증서 점검 대상이 없습니다.", "자세히 보기: '웹사이트' 메뉴")
	}
	sorted := append([]ssl.CertInfo{}, certs...)
	days := func(c ssl.CertInfo) int { return int(c.NotAfter.Sub(now).Hours() / 24) }
	sort.SliceStable(sorted, func(i, j int) bool {
		ei, ej := sorted[i].Status == "error", sorted[j].Status == "error"
		if ei != ej {
			return ei
		}
		return days(sorted[i]) < days(sorted[j])
	})
	soon, failed := 0, 0
	lines := make([]string, 0, len(sorted))
	for _, c := range sorted {
		name := chatLabel(c.Domain)
		if c.Status == "error" {
			failed++
			lines = append(lines, "- "+name+": 점검 실패 (만료일 모름)")
			continue
		}
		left := days(c)
		expiry := c.NotAfter.In(chatKST).Format("2006년 1월 2일")
		if c.NotAfter.Before(now) {
			soon++
			lines = append(lines, fmt.Sprintf("- %s: 만료됨 (%s)", name, expiry))
			continue
		}
		if left <= 30 {
			soon++
		}
		lines = append(lines, fmt.Sprintf("- %s: %d일 남음 (%s 만료)", name, left, expiry))
	}
	first := fmt.Sprintf("%s · 인증서 %d개 중 30일 안에 만료 %d개", head, len(sorted), soon)
	if failed > 0 {
		first += fmt.Sprintf(", 점검 실패 %d개", failed)
	}
	return joinChat(first, strings.Join(capLines(lines), "\n"), footer)
}

const chatHistoryLimit = 10000

func (d *ChatDeps) answerHistory(ctx context.Context, head, q, host string, now time.Time) string {
	p, found, unsupported := chatParsePeriod(q, now)
	if unsupported {
		return joinChat(head+" · 그 기간은 조회할 수 없습니다.", "어제, 지난주, 이번 달, 최근 N일처럼 물어봐 주세요. 자세히 보기: '알림 기록' 메뉴")
	}
	if d.History == nil {
		return joinChat(head+" · 알림 기록 저장소가 꺼져 있어 조회할 수 없습니다.", "자세히 보기: '알림 기록' 메뉴")
	}
	defaulted := false
	if !found {
		p = chatPeriod{now.Add(-24 * time.Hour), now, "최근 24시간"}
		defaulted = true
	}
	records, err := d.History.Search(ctx, db.AlertHistoryFilter{From: p.from, To: p.to, Host: host, Limit: chatHistoryLimit})
	if err != nil {
		log.Printf("[chat] 알림 기록 조회 실패: %v", err)
		return head + " · " + chatFetchFailed
	}
	return renderChatHistory(head, records, p, host, defaulted)
}

func renderChatHistory(head string, records []db.AlertRecord, p chatPeriod, host string, defaulted bool) string {
	crit, warn, info, recovered := 0, 0, 0, 0
	byTarget := map[string]int{}
	for _, r := range records {
		// info 등급에는 복구 말고 인증서 갱신 안내 등도 있어 제목으로 구분한다
		if strings.HasPrefix(r.Subject, "[복구]") {
			recovered++
			continue
		}
		switch r.Severity {
		case "critical":
			crit++
		case "warning":
			warn++
		default:
			info++
		}
		byTarget[chatLabel(r.Host)]++
	}
	total := crit + warn + info
	scope := p.label + " 전체"
	if host != "" {
		scope = p.label + " " + chatLabel(host)
	}
	count := fmt.Sprintf("%d건", total)
	if len(records) >= chatHistoryLimit {
		count += " 이상"
	}

	notes := "알림 건수는 장애 횟수가 아니며, 같은 문제가 30분마다 다시 기록될 수 있습니다. 점검 중인 서버의 알림은 기록되지 않습니다."
	if defaulted {
		notes = "지정한 기간이 없거나 해석하지 못해 최근 24시간 기준입니다. " + notes
	}
	footer := "자세히 보기: '알림 기록' 메뉴"
	if total == 0 {
		line := head + " · " + scope + ": 알림 기록이 없습니다"
		if recovered > 0 {
			line += fmt.Sprintf(" (복구 안내 %d건)", recovered)
		}
		return joinChat(line, notes, footer)
	}

	targets := make([]string, 0, len(byTarget))
	for t := range byTarget {
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool {
		if byTarget[targets[i]] != byTarget[targets[j]] {
			return byTarget[targets[i]] > byTarget[targets[j]]
		}
		return targets[i] < targets[j]
	})
	lines := make([]string, 0, len(targets))
	for _, t := range targets {
		lines = append(lines, fmt.Sprintf("- %s: %d건", t, byTarget[t]))
	}
	first := fmt.Sprintf("%s · %s: 알림 %s (위험 %d · 주의 %d · 정보 %d)", head, scope, count, crit, warn, info)
	if recovered > 0 {
		first += fmt.Sprintf(" · 복구 안내 %d건 별도", recovered)
	}
	return joinChat(first, "**대상별**\n"+strings.Join(capLines(lines), "\n"), notes, footer)
}

type chatInsightGroup struct {
	hosts    []string
	severity loginsight.Severity
	category loginsight.Category
	count    int
}

var chatSeverityRank = map[loginsight.Severity]int{
	loginsight.SeverityCritical: 3, loginsight.SeverityHigh: 2, loginsight.SeverityMedium: 1, loginsight.SeverityLow: 0,
}

var chatSeverityLabel = map[loginsight.Severity]string{
	loginsight.SeverityCritical: "긴급", loginsight.SeverityHigh: "위험", loginsight.SeverityMedium: "주의", loginsight.SeverityLow: "정보",
}

var chatCategoryLabel = map[loginsight.Category]string{
	loginsight.CatAuth: "인증", loginsight.CatNetwork: "네트워크", loginsight.CatDisk: "디스크", loginsight.CatDB: "데이터베이스",
	loginsight.CatApp: "프로그램", loginsight.CatSecurity: "보안", loginsight.CatOther: "기타",
}

// 화면 카드와 같은 키로 묶는다. 요약문은 공격자가 로그로 유도할 수 있어 등급·분류만 쓴다
func groupChatInsights(items []loginsight.Insight, host string) []chatInsightGroup {
	index := map[string]int{}
	var out []chatInsightGroup
	for _, it := range items {
		if host != "" && it.HostName != host {
			continue
		}
		key := it.IncidentKey
		if key == "" {
			key = it.HostName + "|" + it.TemplateHash
		}
		if i, ok := index[key]; ok {
			out[i].count++
			if !slices.Contains(out[i].hosts, it.HostName) {
				out[i].hosts = append(out[i].hosts, it.HostName)
			}
			if chatSeverityRank[it.Severity] > chatSeverityRank[out[i].severity] {
				out[i].severity = it.Severity
			}
			continue
		}
		index[key] = len(out)
		out = append(out, chatInsightGroup{hosts: []string{it.HostName}, severity: it.Severity, category: it.Category, count: 1})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := chatSeverityRank[out[i].severity], chatSeverityRank[out[j].severity]; a != b {
			return a > b
		}
		return out[i].count > out[j].count
	})
	return out
}

func (d *ChatDeps) answerInsights(ctx context.Context, head, host string, now time.Time) string {
	footer := "내용과 조치 방법은 'AI 분석' 메뉴에서 확인해 주세요."
	if d.Insights == nil || d.Health == nil {
		return joinChat(head+" · AI 로그 분석이 꺼져 있어 결과가 없습니다.", "'로그' 메뉴에서 원본 로그를 확인할 수 있습니다.")
	}
	items, err := d.recentInsights(ctx, now)
	if err != nil {
		log.Printf("[chat] 로그 분석 조회 실패: %v", err)
		return head + " · " + chatFetchFailed
	}
	return renderChatInsights(head, items, host, d.Health(), footer)
}

func renderChatInsights(head string, items []loginsight.Insight, host string, health loginsight.Health, footer string) string {
	groups := groupChatInsights(items, host)
	scope := "최근 24시간 전체"
	if host != "" {
		scope = "최근 24시간 " + chatLabel(host)
	}
	stale := ""
	if health.Failing() {
		stale = "AI 로그 분석이 최근 실패하고 있어 새 문제는 반영되지 않았을 수 있습니다."
		if health.LastSuccessAt != nil {
			stale = fmt.Sprintf("AI 로그 분석이 최근 실패하고 있어 %s 이후의 문제는 반영되지 않았을 수 있습니다.", health.LastSuccessAt.In(chatKST).Format("1월 2일 15:04"))
		}
	}
	if len(groups) == 0 {
		return joinChat(
			fmt.Sprintf("%s · AI 로그 분석 %s: 분석 결과가 없습니다", head, scope),
			stale,
			"로그 수집이 안 될 때도 0건으로 보이므로 이상이 없다는 뜻은 아닙니다.",
			footer,
		)
	}

	count := map[loginsight.Severity]int{}
	security := false
	var lines []string
	for _, g := range groups {
		count[g.severity]++
		if g.category == loginsight.CatSecurity || g.category == loginsight.CatAuth {
			security = true
		}
		if g.severity == loginsight.SeverityLow {
			continue
		}
		name := chatLabel(g.hosts[0])
		if n := len(g.hosts); n > 1 {
			name += fmt.Sprintf(" 외 %d대", n-1)
		}
		lines = append(lines, fmt.Sprintf("- %s · %s · %s · %d회 분석", name, chatSeverityLabel[g.severity], chatCategoryLabel[g.category], g.count))
	}
	first := fmt.Sprintf("%s · AI 로그 분석(AI 추정) %s: 문제 %d가지 (긴급 %d · 위험 %d · 주의 %d · 정보 %d)",
		head, scope, len(groups), count[loginsight.SeverityCritical], count[loginsight.SeverityHigh], count[loginsight.SeverityMedium], count[loginsight.SeverityLow])
	body := ""
	if len(lines) > 0 {
		body = strings.Join(capLines(lines), "\n")
	}
	secNote := ""
	if security {
		secNote = "보안 항목의 실제 침해 여부는 이 결과만으로 판단할 수 없습니다."
	}
	return joinChat(first, body, stale, secNote, footer)
}
