package prom

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// promResult는 일반 PromQL instant query 응답.
type promResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// query는 PromQL instant query 실행 + 결과 파싱.
func query(prometheusURL, promql string) (*promResult, error) {
	q := url.QueryEscape(promql)
	resp, err := httpClient.Get(prometheusURL + "/api/v1/query?query=" + q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var r promResult
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("prom 응답 파싱 실패: %w", err)
	}
	return &r, nil
}

// valFloat는 PromQL 결과의 value(스트링) → float64.
func valFloat(v any) float64 {
	s, ok := v.(string)
	if !ok {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// SNMPSnapshot은 한 장비의 통합 메트릭 스냅샷 (Prometheus 기반).
// agent 프록시든 server 직접 폴링이든 둘 다 같은 메트릭 이름으로 적재되어
// 데이터 소스 무관하게 일관 UI 제공.
type SNMPSnapshot struct {
	DeviceName string                  `json:"device_name"`
	Up         bool                    `json:"up"`
	UptimeSec  float64                 `json:"uptime_sec"`
	ResponseMs int64                   `json:"response_ms"`
	SysDescr   string                  `json:"sys_descr,omitempty"`
	LastError  string                  `json:"last_error,omitempty"`
	Interfaces []SNMPInterfaceSnapshot `json:"interfaces,omitempty"`
	Supplies   []SNMPSupplySnapshot    `json:"supplies,omitempty"`
	TotalPages int64                   `json:"total_pages,omitempty"`
}

type SNMPInterfaceSnapshot struct {
	Index  int     `json:"index"`
	Name   string  `json:"name"`
	OperUp bool    `json:"oper_up"`
	InBps  float64 `json:"in_bps"`
	OutBps float64 `json:"out_bps"`
}

type SNMPSupplySnapshot struct {
	Description string  `json:"description"`
	PercentLeft float64 `json:"percent_left"`
}

// SNMPStatusAll은 모든 장비의 Prometheus 기반 통합 스냅샷을 조회 3번으로 모은다 — 장비마다 8번씩 물으면 수백 대에서 화면 주기를 넘김.
// agent 프록시(snmp_*) 또는 server 직접 폴링 둘 다 같은 메트릭 이름으로 적재되어 UI는 어디서 폴링됐는지 신경 안 써도 됨.
func SNMPStatusAll(prometheusURL string) (map[string]*SNMPSnapshot, error) {
	// rate는 메트릭 이름을 지워 in/out을 한 조회로 구분할 수 없음. 수집기가 둘이면 같은 포트가 두 번 나와 sum이 아니라 max
	qs := []string{
		`{__name__=~"snmp_device_up|snmp_device_uptime_seconds|snmp_device_response_ms_milliseconds|snmp_switch_if_oper_up|snmp_printer_supply_percent|snmp_printer_total_pages"}`,
		`max by (device, if_index) (rate(snmp_switch_if_in_bytes_total[2m]))`,
		`max by (device, if_index) (rate(snmp_switch_if_out_bytes_total[2m]))`,
	}
	rs := make([]*promResult, len(qs))
	for i, q := range qs {
		r, err := query(prometheusURL, q)
		if err == nil && r.Status != "success" {
			err = fmt.Errorf("prometheus 응답 상태: %s", r.Status)
		}
		if err != nil {
			if i == 0 {
				return nil, err
			}
			// 포트가 많으면 rate 조회만 시간 제한에 걸릴 수 있음 — 트래픽만 비우고 상태·응답 시간은 살림
			log.Printf("[SNMP] 포트 트래픽 조회 실패: %v", err)
			r = &promResult{}
		}
		rs[i] = r
	}

	snaps := map[string]*SNMPSnapshot{}
	ifs := map[string]map[int]*SNMPInterfaceSnapshot{}
	for _, x := range rs[0].Data.Result {
		dev := x.Metric["device"]
		if dev == "" {
			continue
		}
		s := snaps[dev]
		if s == nil {
			s = &SNMPSnapshot{DeviceName: dev}
			snaps[dev] = s
		}
		v := valFloat(x.Value[1])
		switch x.Metric["__name__"] {
		case "snmp_device_up":
			s.Up = v > 0
			// 응답 실패 때 device_descr가 빈 시계열이 함께 남음 — 빈 값이 설명을 덮지 않게
			if d := x.Metric["device_descr"]; d != "" {
				s.SysDescr = d
			}
		case "snmp_device_uptime_seconds":
			s.UptimeSec = v
		case "snmp_device_response_ms_milliseconds":
			s.ResponseMs = int64(v)
		case "snmp_switch_if_oper_up":
			idx, _ := strconv.Atoi(x.Metric["if_index"])
			if idx == 0 {
				continue
			}
			if ifs[dev] == nil {
				ifs[dev] = map[int]*SNMPInterfaceSnapshot{}
			}
			ifs[dev][idx] = &SNMPInterfaceSnapshot{Index: idx, Name: x.Metric["if_name"], OperUp: v > 0}
		case "snmp_printer_supply_percent":
			s.Supplies = append(s.Supplies, SNMPSupplySnapshot{Description: x.Metric["supply"], PercentLeft: v})
		case "snmp_printer_total_pages":
			s.TotalPages = int64(v)
		}
	}
	for i, r := range rs[1:] {
		for _, x := range r.Data.Result {
			idx, _ := strconv.Atoi(x.Metric["if_index"])
			if ifc := ifs[x.Metric["device"]][idx]; ifc != nil {
				if i == 0 {
					ifc.InBps = valFloat(x.Value[1])
				} else {
					ifc.OutBps = valFloat(x.Value[1])
				}
			}
		}
	}
	for dev, m := range ifs {
		for _, ifc := range m {
			snaps[dev].Interfaces = append(snaps[dev].Interfaces, *ifc)
		}
	}
	return snaps, nil
}

// 수집기가 (서버 시계로) 자기 수집 간격의 3배, 최소 이 시간 넘게 새 샘플을 보내지 않으면 끊긴 것으로 봄 —
// 수집 주기(OWLMON_COLLECT_INTERVAL)를 늘린 에이전트가 주기마다 끊김으로 보이지 않게
const staleCollector = 90 * time.Second

// SNMPSample은 장비 하나의 에이전트 수집 결과 — Up은 살아 있는 수집기 중 하나라도 응답, At은 그중 마지막으로 새 샘플이 들어온 서버 시각
type SNMPSample struct {
	Up bool
	At time.Time
}

// 수집기별로 샘플 시각이 마지막으로 바뀐 때를 서버 시계로 기록 — 에이전트마다 시계가 달라(망분리라 NTP 없음) 샘플 시각끼리 비교할 수 없음.
// 끊긴 수집기의 마지막 값은 remote write에 staleness 표시가 없어 5분간 남으므로 이걸로 걸러 냄
var collectors = struct {
	sync.Mutex
	seen map[string]collectorSeen
	now  func() time.Time
}{seen: map[string]collectorSeen{}, now: time.Now}

type collectorSeen struct {
	t        float64       // 수집기 시계 기준 샘플 시각
	changed  time.Time     // 서버 시계 기준 마지막 변경
	interval time.Duration // 직전 샘플 간격 (수집기 시계)
}

// SNMPUpAll은 모든 장비의 응답 여부를 한 번에 조회 — 장비마다 따로 물으면 수백 대에서 감시 주기를 넘김.
// 살아 있는 수집기가 있는 장비만 들어감(Up=false = 수집기가 무응답을 확인)
func SNMPUpAll(prometheusURL string) (map[string]SNMPSample, error) {
	vals, err := query(prometheusURL, "snmp_device_up")
	if err != nil {
		return nil, err
	}
	ts, err := query(prometheusURL, "timestamp(snmp_device_up)")
	if err != nil {
		return nil, err
	}
	for _, r := range []*promResult{vals, ts} {
		if r.Status != "success" {
			return nil, fmt.Errorf("prometheus 응답 상태: %s", r.Status)
		}
	}
	at := map[string]float64{}
	for _, x := range ts.Data.Result {
		at[labelKey(x.Metric, "__name__")] = valFloat(x.Value[1])
	}
	// 응답 실패 때 device_descr 라벨이 비면서 시계열이 바뀌는데, 직전 시계열(값 1)이 몇 분간 남음 —
	// 수집기(device_descr를 뺀 라벨)마다 가장 최근 샘플만 쓰고, 수집기끼리는 하나라도 응답하면 정상
	type pick struct {
		name string
		t    float64
		up   bool
	}
	byCollector := map[string]pick{}
	for _, x := range vals.Data.Result {
		name := x.Metric["device"]
		if name == "" {
			continue
		}
		k := labelKey(x.Metric, "__name__", "device_descr")
		p := pick{name: name, t: at[labelKey(x.Metric, "__name__")], up: valFloat(x.Value[1]) > 0}
		if prev, ok := byCollector[k]; !ok || p.t > prev.t || (p.t == prev.t && p.up) {
			byCollector[k] = p
		}
	}
	collectors.Lock()
	defer collectors.Unlock()
	now := collectors.now()
	latest := map[string]SNMPSample{}
	for k, p := range byCollector {
		c, ok := collectors.seen[k]
		if !ok || c.t != p.t {
			iv := c.interval
			if ok && p.t > c.t {
				iv = time.Duration((p.t - c.t) * float64(time.Second))
			}
			c = collectorSeen{t: p.t, changed: now, interval: iv}
			collectors.seen[k] = c
		}
		if now.Sub(c.changed) > max(staleCollector, 3*c.interval) {
			continue
		}
		cur := latest[p.name]
		cur.Up = cur.Up || p.up
		if c.changed.After(cur.At) {
			cur.At = c.changed
		}
		latest[p.name] = cur
	}
	for k := range collectors.seen {
		if _, ok := byCollector[k]; !ok {
			delete(collectors.seen, k)
		}
	}
	return latest, nil
}

// SNMPAgentSeen은 하루 안에 에이전트가 수집한 적 있는 장비 — 지금 시계열이 없으면 장비가 아니라 수집 경로(에이전트·Collector)가 끊긴 것.
// 하루짜리 범위 조회라 감시 주기마다 부르지 않음
func SNMPAgentSeen(prometheusURL string) (map[string]bool, error) {
	r, err := query(prometheusURL, "count by (device) (count_over_time(snmp_device_up[1d]))")
	if err != nil {
		return nil, err
	}
	if r.Status != "success" {
		return nil, fmt.Errorf("prometheus 응답 상태: %s", r.Status)
	}
	seen := map[string]bool{}
	for _, x := range r.Data.Result {
		if name := x.Metric["device"]; name != "" {
			seen[name] = true
		}
	}
	return seen, nil
}

func labelKey(m map[string]string, skip ...string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if !slices.Contains(skip, k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + m[k] + "\x00")
	}
	return b.String()
}
