package alert

import (
	"context"
	"strings"
	"testing"
)

func down(host string) Alert {
	return Alert{Host: host, Category: "down", Subject: "[위험] " + host + " 서버 연결 끊김", Body: "호스트: " + host}
}

func TestSuppressChildrenOfDownParent(t *testing.T) {
	cfg := escCfg(10)
	c, _ := newTestChecker(cfg)
	h := &memHistory{}
	c.history = h
	topo := Topology{Parent: map[string]string{"web1": "sw1", "web2": "sw1", "db1": "sw2", "sw1": "core", "sw2": "core"}, Pair: map[string]string{"sw2": "sw2b", "sw2b": "sw2"}}
	c.SetTopology(func(context.Context) (Topology, error) { return topo, nil })

	// 스위치가 먼저 끊김(네트워크 장비 경로) — 하위 서버 끊김은 따로 알리지 않음
	c.SetNetDown("sw1", true)
	c.setDown("web1", true)
	c.setDown("web2", true)
	c.SendDowns("[위험] 서버 %d대 연결 끊김", []Alert{down("web1"), down("web2")})
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("children alerted: %+v", got)
	}
	if len(h.subjects) != 2 || !strings.Contains(h.subjects[0], "sw1 장애 영향") {
		t.Fatalf("history: %v", h.subjects)
	}

	// 같은 주기에 원인과 하위가 함께 오면 원인 알림 본문에 영향 장비
	c2, _ := newTestChecker(cfg)
	c2.SetTopology(func(context.Context) (Topology, error) {
		return Topology{Parent: map[string]string{"a": "gw", "b": "gw"}}, nil
	})
	c2.SendDowns("[위험] 서버 %d대 연결 끊김", []Alert{down("gw"), down("a"), down("b")})
	got := drain(c2.notifier)
	if len(got) != 1 || !strings.Contains(got[0].Subject, "gw") || !strings.Contains(got[0].Body, "함께 끊긴 하위 장비 2대") {
		t.Fatalf("cause notice: %+v", got)
	}

	// 짝이 살아 있으면 상위가 끊겨도 원인이 아님 — 하위 장비는 따로 알림
	c.SetNetDown("sw2", true)
	c.SendDowns("[위험] 서버 %d대 연결 끊김", []Alert{down("db1")})
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "db1") {
		t.Fatalf("pair alive: %+v", got)
	}

	// 스위치 복구: web1은 같이 살아나 복구 알림 생략, web2는 아직 끊겨 있어 이제 알림
	c.SetNetDown("sw1", false)
	c.setDown("web1", false)
	c.SendUps("[복구] 서버 %d대 연결 복구", []Alert{{Host: "web1", Category: "down", Subject: "[복구] web1 서버 연결 복구"}})
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("quiet recovery alerted: %+v", got)
	}
	c.reviewQuiet()
	got = drain(c.notifier)
	if len(got) != 1 || !strings.Contains(got[0].Subject, "web2") || !strings.Contains(got[0].Body, "아직 끊겨") {
		t.Fatalf("late alert: %+v", got)
	}
	c.reviewQuiet()
	if got := drain(c.notifier); len(got) != 0 {
		t.Fatalf("late alert repeated: %+v", got)
	}

	// 점검 중인 상위 장비는 원인으로 보지 않음 (화면과 같은 기준)
	c3, _ := newTestChecker(cfg)
	c3.SetTopology(func(context.Context) (Topology, error) { return Topology{Parent: map[string]string{"x": "sw9"}}, nil })
	c3.SetNetDown("sw9", true)
	c3.SetMaintenance("sw9", true)
	c3.SendDowns("[위험] 서버 %d대 연결 끊김", []Alert{down("x")})
	if got := drain(c3.notifier); len(got) != 1 {
		t.Fatalf("maintenance parent: %+v", got)
	}

	// 구성도를 넘기지 않으면(저장된 구성도 없음) 억제하지 않음
	c4, _ := newTestChecker(cfg)
	c4.SetNetDown("sw1", true)
	c4.SendDowns("[위험] 서버 %d대 연결 끊김", []Alert{down("web1")})
	if got := drain(c4.notifier); len(got) != 1 {
		t.Fatalf("no topology: %+v", got)
	}
}

func TestSuppressEdges(t *testing.T) {
	cfg := escCfg(10)
	topo := Topology{Parent: map[string]string{"a": "sw", "b": "sw", "c": "sw", "d": "sw", "e": "sw"}}
	mk := func() *Checker {
		c, _ := newTestChecker(cfg)
		c.SetTopology(func(context.Context) (Topology, error) { return topo, nil })
		return c
	}

	// 원인 행 확인이 하위 장비 확인까지 눌렀어도, 원인 복구 뒤 아직 끊긴 하위 장비는 알림
	c := mk()
	c.SetNetDown("sw", true)
	c.setDown("a", true)
	c.SendDowns("%d", []Alert{down("a")})
	c.Ack("a", "down", "critical", "kim")
	c.SetNetDown("sw", false)
	drain(c.notifier)
	c.reviewQuiet()
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "a") {
		t.Fatalf("acked child late alert: %+v", got)
	}

	// 이미 끊김 알림을 보낸 장비는 나중에 상위가 끊겨도 복구 알림을 보냄
	c = mk()
	c.SendDowns("%d", []Alert{down("b")})
	c.SetNetDown("sw", true)
	c.SendDowns("%d", []Alert{down("b")}) // 30분 뒤 재알림 주기
	drain(c.notifier)
	c.SendUps("%d", []Alert{{Host: "b", Category: "down", Subject: "[복구] b"}})
	if got := drain(c.notifier); len(got) != 1 {
		t.Fatalf("recovery after sent: %+v", got)
	}

	// 원인 장비를 점검으로 돌리면 하위 여러 대의 늦은 알림을 한 통으로
	c = mk()
	c.SetNetDown("sw", true)
	hs := []Alert{down("a"), down("b"), down("c"), down("d"), down("e")}
	for _, h := range hs {
		c.setDown(h.Host, true)
	}
	c.SendDowns("%d", hs)
	drain(c.notifier)
	c.SetMaintenance("sw", true)
	c.reviewQuiet()
	if got := drain(c.notifier); len(got) != 1 || !strings.Contains(got[0].Subject, "5대") {
		t.Fatalf("grouped late: %+v", got)
	}
}
