package alert

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// Topology는 저장된 구성도의 연결 — 이름 → 위쪽 장비, 이름 → 이중화 짝
type Topology struct {
	Parent map[string]string
	Pair   map[string]string
}

// 상위 장비가 끊겨 함께 끊긴 하위 장비는 따로 알리지 않고 원인 장비 알림에 묶음 — 스위치 하나에 메일·문자 수백 통이 나가지 않게.
// 화면(useFleet causedBy)과 같은 기준: 점검 중인 장비는 원인으로 보지 않고, 짝이 살아 있으면 끊긴 게 아님
type suppressor struct {
	mu      sync.Mutex
	load    func(context.Context) (Topology, error)
	topo    Topology
	at      time.Time
	netDown map[string]bool  // SNMP 장애 중 장비
	quiet   map[string]Alert // 상위 장애로 끊김 알림을 보내지 않은 장비 — 복구 알림도 보내지 않고, 원인이 풀린 뒤에도 끊겨 있으면 그때 보냄
	sent    map[string]bool  // 끊김 알림을 이미 보낸 장비 — 나중에 상위가 끊겨도 생략으로 바꾸면 복구 알림이 빠짐
}

const topologyTTL = time.Minute

// SetTopology는 구성도 읽는 함수를 넘김 (Start 전에). 저장된 구성도가 없으면 억제하지 않음 — 화면의 자동 배치는 추정이라 알림을 막는 근거로 쓰지 않음
func (c *Checker) SetTopology(load func(context.Context) (Topology, error)) {
	c.sup.load = load
}

// SetNetDown은 네트워크 장비 장애 시작·끝을 알려 줌 — 서버 끊김 알림이 그 장비를 원인으로 묶을 수 있게
func (c *Checker) SetNetDown(name string, down bool) {
	c.sup.mu.Lock()
	defer c.sup.mu.Unlock()
	if c.sup.netDown == nil {
		c.sup.netDown = map[string]bool{}
	}
	if down {
		c.sup.netDown[name] = true
	} else {
		delete(c.sup.netDown, name)
	}
}

func (c *Checker) topology() Topology {
	s := &c.sup
	if s.load == nil || (!s.at.IsZero() && time.Since(s.at) < topologyTTL) {
		return s.topo
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	t, err := s.load(ctx)
	if err != nil {
		log.Printf("구성도 읽기 실패 — 지난 값으로 묶음 판단: %v", err) // 못 읽으면 억제를 멈추기보다 지난 연결을 씀
	} else {
		s.topo = t
	}
	s.at = time.Now()
	return s.topo
}

// downSet은 지금 끊긴 장비 — 서버 끊김, 네트워크 장비 장애. 점검 중인 장비는 원인에서 뺌. 호출 측이 sup.mu를 잡고 있어야 함
func (c *Checker) downSet(extra []Alert) map[string]bool {
	down := map[string]bool{}
	c.escMu.Lock()
	for h := range c.downHosts {
		down[h] = true
	}
	c.escMu.Unlock()
	for n := range c.sup.netDown {
		down[n] = true
	}
	for _, a := range extra {
		down[a.Host] = true
	}
	for n := range down {
		if c.state.IsInMaintenance(n) {
			delete(down, n)
		}
	}
	return down
}

// causeOf는 끊긴 가장 위쪽 장비 — 그 장비의 짝이 살아 있으면 원인으로 보지 않음
func causeOf(name string, t Topology, down map[string]bool) string {
	cause := ""
	for p, hops := t.Parent[name], 0; p != "" && hops < 50; p, hops = t.Parent[p], hops+1 {
		if down[p] && (t.Pair[p] == "" || down[t.Pair[p]]) {
			cause = p
		}
	}
	return cause
}

// SendDowns는 끊김 알림을 원인별로 나눠 보냄: 원인 장비 알림에 영향 장비 목록을 붙이고, 하위 장비는 이력만 남김
func (c *Checker) SendDowns(subjectFmt string, alerts []Alert) {
	c.sup.mu.Lock()
	t := c.topology()
	down := c.downSet(alerts)
	var send []Alert
	kids := map[string][]string{}
	if c.sup.sent == nil {
		c.sup.sent = map[string]bool{}
	}
	for _, a := range alerts {
		if p := causeOf(a.Host, t, down); p != "" && !c.sup.sent[a.Host] {
			if c.sup.quiet == nil {
				c.sup.quiet = map[string]Alert{}
			}
			c.sup.quiet[a.Host] = a
			kids[p] = append(kids[p], a.Host)
			c.saveHistory(a.Host, a.Category, "critical", a.Subject+" — "+p+" 장애 영향(알림 생략)", a.Body)
			continue
		}
		c.sup.sent[a.Host] = true
		send = append(send, a)
	}
	c.sup.mu.Unlock()
	for i := range send {
		if k := kids[send[i].Host]; len(k) > 0 {
			send[i].Body += impactLine(k)
		}
	}
	c.SendGroup("critical", subjectFmt, send)
}

// SendUps는 복구 알림을 보냄 — 끊김 알림을 생략한 장비는 복구도 이력만
func (c *Checker) SendUps(subjectFmt string, alerts []Alert) {
	var send []Alert
	c.sup.mu.Lock()
	for _, a := range alerts {
		delete(c.sup.sent, a.Host)
		if _, ok := c.sup.quiet[a.Host]; ok {
			delete(c.sup.quiet, a.Host)
			c.saveHistory(a.Host, a.Category, "info", a.Subject+" (끊김 알림 생략했던 장비)", a.Body)
			continue
		}
		send = append(send, a)
	}
	c.sup.mu.Unlock()
	c.SendGroup("info", subjectFmt, send)
}

// ForgetQuiet는 지우거나 이름을 바꾼 장비의 생략 기록을 버림
func (c *Checker) ForgetQuiet(name string) {
	c.sup.mu.Lock()
	delete(c.sup.quiet, name)
	delete(c.sup.sent, name)
	c.sup.mu.Unlock()
}

// reviewQuiet는 원인 장비가 살아났는데 아직 끊긴 하위 장비를 이제 알림 — 그 장비 자체의 장애였음. 체커 주기마다
func (c *Checker) reviewQuiet() {
	c.sup.mu.Lock()
	if len(c.sup.quiet) == 0 {
		c.sup.mu.Unlock()
		return
	}
	t := c.topology()
	down := c.downSet(nil)
	if c.sup.sent == nil {
		c.sup.sent = map[string]bool{}
	}
	var late []Alert
	for name, a := range c.sup.quiet {
		if causeOf(name, t, down) != "" {
			continue
		}
		delete(c.sup.quiet, name)
		if down[name] {
			a.Body += "\n\n위쪽 장비가 복구됐지만 이 장비는 아직 끊겨 있습니다."
			c.sup.sent[name] = true
			late = append(late, a)
		}
	}
	c.sup.mu.Unlock()
	sort.Slice(late, func(i, j int) bool { return late[i].Host < late[j].Host })
	for _, a := range late {
		// 화면에서 원인 장비 행을 확인하면 하위 장비 확인도 함께 눌림 — 그 확인으로 이 장비 자체의 장애가 묻히지 않게 풂
		c.ClearAck(a.Host, a.Category, "critical")
	}
	// 원인 장비를 점검으로 돌리거나 지우면 하위 수백 대가 한꺼번에 여기로 옴 — 묶어서 보냄
	c.SendGroup("critical", "[위험] 위쪽 장비 복구 후에도 끊긴 장비 %d대", late)
}

func impactLine(names []string) string {
	sort.Strings(names)
	shown := names
	if len(shown) > 20 {
		shown = shown[:20]
	}
	more := ""
	if len(names) > len(shown) {
		more = fmt.Sprintf(" 외 %d대", len(names)-len(shown))
	}
	return fmt.Sprintf("\n\n함께 끊긴 하위 장비 %d대 (따로 알리지 않음): %s%s", len(names), strings.Join(shown, ", "), more)
}
