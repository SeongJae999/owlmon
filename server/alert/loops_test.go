package alert

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// 가짜 Prometheus — 받은 쿼리를 세고, 쿼리 문자열에 맞는 응답을 돌려줌
type fakeProm struct {
	mu      sync.Mutex
	queries map[string]int
	reply   func(q string) string // 결과 배열 JSON, 빈 문자열이면 빈 결과
}

func (f *fakeProm) start(t *testing.T) string {
	f.queries = map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.FormValue("query")
		f.mu.Lock()
		f.queries[q]++
		f.mu.Unlock()
		res := f.reply(q)
		if res == "" {
			res = "[]"
		}
		fmt.Fprintf(w, `{"status":"success","data":{"result":%s}}`, res)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func series(label string, vals ...string) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf(`{"metric":{"%s":"%s"},"value":[0,"1"]}`, label, v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// 한 주기에 CPU·메모리·디스크 쿼리는 임계 판정과 이상탐지·예측이 함께 써서 한 번씩만, 급증은 호스트별 최댓값으로 집계해 받음
func TestCheckQueriesOncePerCycle(t *testing.T) {
	f := &fakeProm{reply: func(q string) string {
		switch {
		case q == "up":
			return series("job", "x")
		case strings.Contains(q, "system_cpu_usage_percent") && strings.HasPrefix(q, "max("):
			return series("host_name", "web-01")
		case strings.Contains(q, "system_disk_usage_percent) by"):
			return `[{"metric":{"host_name":"web-01","mountpoint":"/"},"value":[0,"10"]}]`
		}
		return ""
	}}
	url := f.start(t)
	c := NewChecker(url, nil, &memConfig{AlertConfig{Enabled: true, CPUThreshold: 90, MemThreshold: 95, DiskWarn: 85, DiskCrit: 90}}, nil)

	c.check()

	for _, q := range []string{
		"max(system_cpu_usage_percent) by (host_name)",
		"max(system_memory_usage_percent) by (host_name)",
		"max(system_disk_usage_percent) by (host_name, mountpoint)",
		"max by (host_name)(delta(system_disk_usage_percent[1h]))",
		"max by (host_name)(delta(system_memory_usage_percent[1h]))",
	} {
		if f.queries[q] != 1 {
			t.Errorf("%q 조회 %d번 — 1번이어야 함", q, f.queries[q])
		}
	}
}

type countingConfig struct {
	mu   sync.Mutex
	c    AlertConfig
	gets int
}

func (m *countingConfig) Get() AlertConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	return m.c
}
func (m *countingConfig) Set(c AlertConfig) error { m.mu.Lock(); m.c = c; m.mu.Unlock(); return nil }

// 장애 폭주 때 알림마다 설정·위치를 DB에서 읽지 않되, 설정 저장(InvalidateConfig)은 바로 반영돼야 함
func TestConfigAndLocationCached(t *testing.T) {
	mc := &countingConfig{c: AlertConfig{Enabled: true}}
	c := NewChecker("", &EmailConfig{To: []string{"ops@x.kr"}}, mc, nil)
	locates := 0
	c.SetLocator(func(string) string { locates++; return "본관 2층" })

	var alerts []Alert
	for i := range 50 {
		alerts = append(alerts, Alert{Host: "sw1", Category: "snmp", Subject: fmt.Sprintf("[위험] %d", i)})
	}
	c.SendGroup("critical", "[위험] %d대", alerts)
	for range 50 {
		c.SendAlert("sw1", "snmp", "critical", "[위험] sw1", "장비: sw1")
	}
	if mc.gets != 1 || locates != 1 {
		t.Fatalf("설정 조회 %d번·위치 조회 %d번 — 각 1번이어야 함", mc.gets, locates)
	}
	if got := drain(c.notifier); len(got) == 0 || !strings.Contains(got[len(got)-1].Body, "위치: 본관 2층") {
		t.Fatalf("캐시된 위치가 본문에 없음: %+v", got)
	}

	_ = mc.Set(AlertConfig{Enabled: false})
	c.InvalidateConfig()
	c.SendAlert("sw2", "snmp", "critical", "[위험] sw2", "장비: sw2")
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("알림을 끈 설정이 바로 반영되지 않음: %+v", got)
	}
}
