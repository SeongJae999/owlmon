package prom

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type series struct{ labels, val, ts string }

func fakeProm(t *testing.T, cur *[]series) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		var parts []string
		switch {
		case strings.HasPrefix(q, "count by"):
			parts = []string{`{"metric":{"device":"sw1"},"value":[100,"8"]}`, `{"metric":{"device":"sw3"},"value":[100,"3"]}`}
		case strings.HasPrefix(q, "timestamp("):
			for _, s := range *cur {
				parts = append(parts, `{"metric":{`+s.labels+`},"value":[100,"`+s.ts+`"]}`)
			}
		default:
			for _, s := range *cur {
				parts = append(parts, `{"metric":{"__name__":"snmp_device_up",`+s.labels+`},"value":[100,"`+s.val+`"]}`)
			}
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` + strings.Join(parts, ",") + `]}}`))
	}))
}

func TestSNMPUpAllLatest(t *testing.T) {
	now := time.Unix(1000, 0)
	collectors.now = func() time.Time { return now }
	defer func() { collectors.now = time.Now; collectors.seen = map[string]collectorSeen{} }()

	// sw1: 같은 수집기의 예전 시계열(값 1)과 새 시계열(값 0). sw2: 두 수집기 중 하나만 응답 — 시계가 2분 차이 나도 OR.
	// sw3: 수집기 b는 시계가 늦을 뿐 살아 있고, 수집기 c는 끊김(샘플 시각이 안 바뀜)
	cur := []series{
		{`"device":"sw1","job":"a","device_descr":"Cisco"`, "1", "40"},
		{`"device":"sw1","job":"a","device_descr":""`, "0", "95"},
		{`"device":"sw2","job":"a"`, "0", "99"},
		{`"device":"sw2","job":"b"`, "1", "-21"},
		{`"device":"sw3","job":"c"`, "1", "50"},
	}
	srv := fakeProm(t, &cur)
	defer srv.Close()
	latest, err := SNMPUpAll(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := latest["sw1"]; !ok || s.Up || !s.At.Equal(now) {
		t.Errorf("sw1 should be down by latest sample: %+v %v", s, ok)
	}
	if s := latest["sw2"]; !s.Up {
		t.Errorf("sw2 should be up (OR across collectors despite clock skew): %+v", s)
	}

	// 2분 뒤: a·b는 새 샘플, c는 그대로 — c의 마지막 값 1은 판정에서 빠짐(수집기가 하나뿐이어도)
	now = now.Add(2 * time.Minute)
	cur[1].ts, cur[2].ts, cur[3].ts = "215", "219", "99"
	if latest, err = SNMPUpAll(srv.URL); err != nil {
		t.Fatal(err)
	}
	if s := latest["sw2"]; !s.Up || !s.At.Equal(now) {
		t.Errorf("sw2 after: %+v", s)
	}
	if s, ok := latest["sw3"]; ok {
		t.Errorf("sw3 stale single collector should be dropped: %+v", s)
	}

	// 수집 주기가 2분인 수집기(sw4)는 2분 30초 동안 새 샘플이 없어도 끊김이 아님 (자기 간격의 3배)
	cur = []series{{`"device":"sw4","job":"slow"`, "1", "0"}}
	SNMPUpAll(srv.URL)
	now = now.Add(2 * time.Minute)
	cur[0].ts = "120"
	SNMPUpAll(srv.URL)
	now = now.Add(150 * time.Second)
	if latest, _ = SNMPUpAll(srv.URL); !latest["sw4"].Up {
		t.Errorf("slow collector dropped: %+v", latest)
	}

	seen, err := SNMPAgentSeen(srv.URL)
	if err != nil || !seen["sw1"] || !seen["sw3"] || seen["sw2"] {
		t.Errorf("seen: %v %v", seen, err)
	}
}
