package prom

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSNMPStatusAll(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query().Get("query")
		var rows []string
		switch {
		case strings.Contains(q, "in_bytes"):
			rows = []string{`{"metric":{"device":"sw1","if_index":"1"},"value":[0,"100"]}`, `{"metric":{"device":"sw1","if_index":"9"},"value":[0,"5"]}`}
		case strings.Contains(q, "out_bytes"):
			rows = []string{`{"metric":{"device":"sw1","if_index":"1"},"value":[0,"200"]}`}
		default:
			rows = []string{
				`{"metric":{"__name__":"snmp_device_up","device":"sw1","device_descr":"Cisco"},"value":[0,"1"]}`,
				`{"metric":{"__name__":"snmp_device_up","device":"sw1","device_descr":""},"value":[0,"1"]}`,
				`{"metric":{"__name__":"snmp_device_uptime_seconds","device":"sw1"},"value":[0,"3600"]}`,
				`{"metric":{"__name__":"snmp_device_response_ms_milliseconds","device":"sw1"},"value":[0,"12"]}`,
				`{"metric":{"__name__":"snmp_switch_if_oper_up","device":"sw1","if_index":"1","if_name":"Gi0/1"},"value":[0,"1"]}`,
				`{"metric":{"__name__":"snmp_switch_if_oper_up","device":"sw1","if_index":"2","if_name":"Gi0/2"},"value":[0,"0"]}`,
				`{"metric":{"__name__":"snmp_device_up","device":"pr1"},"value":[0,"0"]}`,
				`{"metric":{"__name__":"snmp_printer_supply_percent","device":"pr1","supply":"Black"},"value":[0,"40"]}`,
				`{"metric":{"__name__":"snmp_printer_supply_percent","device":"pr1","supply":"Cyan"},"value":[0,"70"]}`,
				`{"metric":{"__name__":"snmp_printer_total_pages","device":"pr1"},"value":[0,"1234"]}`,
			}
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` + strings.Join(rows, ",") + `]}}`))
	}))
	defer srv.Close()

	snaps, err := SNMPStatusAll(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("장비 수와 무관하게 3번: %d", n)
	}
	sw := snaps["sw1"]
	if sw == nil || !sw.Up || sw.SysDescr != "Cisco" || sw.UptimeSec != 3600 || sw.ResponseMs != 12 {
		t.Fatalf("sw1: %+v", sw)
	}
	slices.SortFunc(sw.Interfaces, func(a, b SNMPInterfaceSnapshot) int { return a.Index - b.Index })
	want := []SNMPInterfaceSnapshot{{Index: 1, Name: "Gi0/1", OperUp: true, InBps: 100, OutBps: 200}, {Index: 2, Name: "Gi0/2"}}
	if !slices.Equal(sw.Interfaces, want) {
		t.Errorf("인터페이스: %+v", sw.Interfaces)
	}
	pr := snaps["pr1"]
	if pr == nil || pr.Up || pr.TotalPages != 1234 || len(pr.Supplies) != 2 || pr.Supplies[1].PercentLeft != 70 || len(pr.Interfaces) != 0 {
		t.Fatalf("pr1: %+v", pr)
	}
}
