package prom

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	resp, err := http.Get(prometheusURL + "/api/v1/query?query=" + q)
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
	DeviceName  string                    `json:"device_name"`
	Up          bool                      `json:"up"`
	UptimeSec   float64                   `json:"uptime_sec"`
	ResponseMs  int64                     `json:"response_ms"`
	SysDescr    string                    `json:"sys_descr,omitempty"`
	LastError   string                    `json:"last_error,omitempty"`
	Interfaces  []SNMPInterfaceSnapshot   `json:"interfaces,omitempty"`
	Supplies    []SNMPSupplySnapshot      `json:"supplies,omitempty"`
	TotalPages  int64                     `json:"total_pages,omitempty"`
}

type SNMPInterfaceSnapshot struct {
	Index   int     `json:"index"`
	Name    string  `json:"name"`
	OperUp  bool    `json:"oper_up"`
	InBps   float64 `json:"in_bps"`
	OutBps  float64 `json:"out_bps"`
}

type SNMPSupplySnapshot struct {
	Description string  `json:"description"`
	PercentLeft float64 `json:"percent_left"`
}

// SNMPStatusForDevice는 한 장비의 Prometheus 기반 통합 스냅샷을 반환합니다.
// agent 프록시(snmp_*) 또는 server 직접 폴링 둘 다 같은 메트릭 이름으로
// 적재되어 UI는 어디서 폴링됐는지 신경 안 써도 됨.
func SNMPStatusForDevice(prometheusURL, deviceName string) (*SNMPSnapshot, error) {
	snap := &SNMPSnapshot{DeviceName: deviceName}
	devSel := fmt.Sprintf(`{device=%q}`, deviceName)

	// 1) up + uptime + response + descr (한 장비 한 row씩)
	if r, err := query(prometheusURL, "snmp_device_up"+devSel); err == nil {
		for _, x := range r.Data.Result {
			snap.Up = valFloat(x.Value[1]) > 0
			snap.SysDescr = x.Metric["device_descr"]
			break
		}
	}
	if r, err := query(prometheusURL, "snmp_device_uptime_seconds"+devSel); err == nil {
		for _, x := range r.Data.Result {
			snap.UptimeSec = valFloat(x.Value[1])
			break
		}
	}
	if r, err := query(prometheusURL, "snmp_device_response_ms_milliseconds"+devSel); err == nil {
		for _, x := range r.Data.Result {
			snap.ResponseMs = int64(valFloat(x.Value[1]))
			break
		}
	}

	// 2) 스위치 인터페이스 (snmp_switch_if_*) — Prometheus rate로 bps 계산
	ifsByIndex := map[int]*SNMPInterfaceSnapshot{}
	if r, err := query(prometheusURL, "snmp_switch_if_oper_up"+devSel); err == nil {
		for _, x := range r.Data.Result {
			idx, _ := strconv.Atoi(x.Metric["if_index"])
			if idx == 0 {
				continue
			}
			ifsByIndex[idx] = &SNMPInterfaceSnapshot{
				Index:  idx,
				Name:   x.Metric["if_name"],
				OperUp: valFloat(x.Value[1]) > 0,
			}
		}
	}
	if r, err := query(prometheusURL, `rate(snmp_switch_if_in_bytes_total`+devSel+`[2m])`); err == nil {
		for _, x := range r.Data.Result {
			idx, _ := strconv.Atoi(x.Metric["if_index"])
			if s, ok := ifsByIndex[idx]; ok {
				s.InBps = valFloat(x.Value[1])
			}
		}
	}
	if r, err := query(prometheusURL, `rate(snmp_switch_if_out_bytes_total`+devSel+`[2m])`); err == nil {
		for _, x := range r.Data.Result {
			idx, _ := strconv.Atoi(x.Metric["if_index"])
			if s, ok := ifsByIndex[idx]; ok {
				s.OutBps = valFloat(x.Value[1])
			}
		}
	}
	for _, s := range ifsByIndex {
		snap.Interfaces = append(snap.Interfaces, *s)
	}

	// 3) 프린터 토너/잉크
	if r, err := query(prometheusURL, "snmp_printer_supply_percent"+devSel); err == nil {
		for _, x := range r.Data.Result {
			snap.Supplies = append(snap.Supplies, SNMPSupplySnapshot{
				Description: x.Metric["supply"],
				PercentLeft: valFloat(x.Value[1]),
			})
		}
	}
	if r, err := query(prometheusURL, "snmp_printer_total_pages"+devSel); err == nil {
		for _, x := range r.Data.Result {
			snap.TotalPages = int64(valFloat(x.Value[1]))
			break
		}
	}

	return snap, nil
}
