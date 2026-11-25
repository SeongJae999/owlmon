package handler

import "testing"

func TestNormalizeWorkOrder(t *testing.T) {
	str := func(v string) *string { return &v }
	cases := []struct {
		name string
		in   workOrderInput
		ok   bool
	}{
		{"정상", workOrderInput{HostName: " web-01 ", Title: " 디스크 교체 ", DueDate: str("2026-12-31")}, true},
		{"기한 없음", workOrderInput{HostName: "web-01", Title: "점검"}, true},
		{"빈 기한은 기한 없음", workOrderInput{HostName: "web-01", Title: "점검", DueDate: str(" ")}, true},
		{"장비 없음", workOrderInput{Title: "점검"}, false},
		{"할 일 없음", workOrderInput{HostName: "web-01", Title: "  "}, false},
		{"날짜 형식 틀림", workOrderInput{HostName: "web-01", Title: "점검", DueDate: str("12/31")}, false},
		{"없는 날짜", workOrderInput{HostName: "web-01", Title: "점검", DueDate: str("2026-02-30")}, false},
	}
	for _, c := range cases {
		wo, msg := normalizeWorkOrder(c.in)
		if (msg == "") != c.ok {
			t.Errorf("%s: msg=%q", c.name, msg)
		}
		if c.ok && (wo.HostName != "web-01" || wo.Title == "" || wo.Title[0] == ' ') {
			t.Errorf("%s: 공백 정리 안 됨 %+v", c.name, wo)
		}
	}
	if wo, _ := normalizeWorkOrder(workOrderInput{HostName: "a", Title: "b", DueDate: str(" "), AlertRef: str("")}); wo.DueDate != nil || wo.AlertRef != nil {
		t.Errorf("빈 값은 null이어야 함: %+v", wo)
	}
}
