package prom

import (
	"sync"
	"time"
)

// 연속 세 번 수집이 없으면 끊김 — 한 번 늦은 전송(묶음 전송 5초·재시도)으로 끊김이 나지 않게. 기본 수집 15초면 45초
const (
	DownMissed = 3
	DownMin    = 45 * time.Second
	// 처음 볼 때 이미 이만큼 오래된 샘플이면 그 시각부터 셈 — 서버를 다시 켠 직후 이미 끊긴 서버가 45초 동안 살아 보이지 않게.
	// 이 경우만 에이전트 시계를 믿음(예전 판정과 같은 2분 허용)
	staleOnFirstSight = 2 * time.Minute
)

// HostLiveness는 서버별 샘플 시각이 마지막으로 바뀐 때를 서버 시계로 기록함.
// 샘플 시각은 에이전트 시계라 '지금 − 샘플 시각'으로 판정하면 NTP 없는 망분리 PC의 시계 차이가 끊김·지연으로 보임 —
// 값이 바뀐 순간을 서버가 직접 보고 그 뒤로 흐른 시간만 씀
type HostLiveness struct {
	mu   sync.Mutex
	seen map[string]liveSample
}

type liveSample struct {
	ts       float64       // 에이전트 시계의 샘플 시각
	changed  time.Time     // 그 값이 바뀐 것을 처음 본 서버 시각
	interval time.Duration // 직전 샘플 간격 (에이전트 시계끼리라 차이는 정확)
}

// HostState는 한 서버의 수집 상태. Known=false면 최근 5분(조회 범위) 안에 샘플이 없음 — 끊긴 지 오래됨
type HostState struct {
	Known    bool
	Age      time.Duration
	Interval time.Duration
	Down     bool
}

// Observe는 호스트별 최신 샘플 시각(timestamp())을 받아 기록을 갱신. 결과에서 빠진 호스트는 잊음
func (l *HostLiveness) Observe(ts map[string]float64, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]liveSample{}
	}
	for host, t := range ts {
		prev, ok := l.seen[host]
		if ok && prev.ts == t {
			continue
		}
		iv := prev.interval
		if ok && t > prev.ts {
			iv = time.Duration((t - prev.ts) * float64(time.Second))
		}
		changed := now
		if age := now.Sub(time.Unix(0, int64(t*float64(time.Second)))); !ok && age > staleOnFirstSight {
			changed = now.Add(-age)
		}
		l.seen[host] = liveSample{ts: t, changed: changed, interval: iv}
	}
	for host := range l.seen {
		if _, ok := ts[host]; !ok {
			delete(l.seen, host)
		}
	}
}

// DownAfter는 수집 간격에 맞춘 끊김 기준 — 수집 주기를 늘린 에이전트(OWLMON_COLLECT_INTERVAL)가 늘 끊김으로 보이지 않게
func DownAfter(interval time.Duration) time.Duration {
	return max(DownMin, DownMissed*interval)
}

func (l *HostLiveness) State(host string, now time.Time) HostState {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.seen[host]
	if !ok {
		return HostState{Down: true}
	}
	age := max(0, now.Sub(s.changed))
	return HostState{Known: true, Age: age, Interval: s.interval, Down: age > DownAfter(s.interval)}
}

// Hosts는 지금 기록이 있는 호스트 목록
func (l *HostLiveness) Hosts() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.seen))
	for h := range l.seen {
		out = append(out, h)
	}
	return out
}
