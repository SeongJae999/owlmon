package collector

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/process"
)

// 서버 알림은 "디스크가 찼다"만 알고 무엇이 채우는지 몰라 관제사가 손쓸 수 없었다.
// 디스크가 빠르게 찰 때만 그 순간 쓰기가 많은 프로세스를 재서 로그 한 줄로 보낸다 (평소엔 사용률만 읽음).
const (
	diskWatchEvery   = time.Minute
	diskWatchSamples = 10  // 10분 전 사용률과 비교
	diskWatchRise    = 1.0 // %p — 10분 +1%p(시간당 약 +6%p)면 서버 급증 알림(+5%p/1h)보다 먼저 남는다
	diskWatchMeasure = 5 * time.Second
	topWriters       = 3
)

// StartDiskWatch는 디스크 급증 감시를 시작합니다. report는 서버로 보낼 로그 한 줄을 받습니다.
func StartDiskWatch(ctx context.Context, report func(line string)) {
	go func() {
		w := diskRise{hist: map[string][]float64{}}
		t := time.NewTicker(diskWatchEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			_ = forEachDisk(func(p disk.PartitionStat, u *disk.UsageStat) {
				from, ok := w.observe(p.Mountpoint, u.UsedPercent)
				if !ok {
					return
				}
				ws, readable, total := topDiskWriters(ctx, diskWatchMeasure)
				report(describeRise(p.Mountpoint, from, u.UsedPercent, ws, readable, total))
			})
		}
	}()
}

// diskRise는 마운트별 최근 사용률을 기억해 diskWatchSamples분 사이 diskWatchRise 이상 오르면 알린다.
type diskRise struct{ hist map[string][]float64 }

func (d diskRise) observe(mount string, pct float64) (from float64, rose bool) {
	h := append(d.hist[mount], pct)
	if len(h) > diskWatchSamples+1 {
		h = h[1:]
	}
	d.hist[mount] = h
	if len(h) == diskWatchSamples+1 && pct-h[0] >= diskWatchRise {
		d.hist[mount] = h[len(h)-1:] // 같은 증가를 매분 다시 보내지 않게 기준을 지금으로 옮김
		return h[0], true
	}
	return 0, false
}

type diskWriter struct {
	pid             int32
	name, user, cwd string
	bytesPerSec     float64
}

// topDiskWriters는 d 동안 디스크에 가장 많이 쓴 프로세스를 잰다.
// 다른 사용자 프로세스의 쓰기량은 root나 CAP_SYS_PTRACE가 있어야 읽혀서 읽은 개수(readable)도 함께 돌려준다.
func topDiskWriters(ctx context.Context, d time.Duration) (ws []diskWriter, readable, total int) {
	first, total := writeBytes(ctx)
	select {
	case <-ctx.Done():
		return nil, len(first), total
	case <-time.After(d):
	}
	second, _ := writeBytes(ctx)
	for pid, b := range second {
		if a, ok := first[pid]; ok && b > a {
			ws = append(ws, diskWriter{pid: pid, bytesPerSec: float64(b-a) / d.Seconds()})
		}
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].bytesPerSec > ws[j].bytesPerSec })
	if len(ws) > topWriters {
		ws = ws[:topWriters]
	}
	for i := range ws {
		p := &process.Process{Pid: ws[i].pid}
		ws[i].name, _ = p.NameWithContext(ctx)
		ws[i].user, _ = p.UsernameWithContext(ctx)
		ws[i].cwd, _ = p.CwdWithContext(ctx)
	}
	return ws, len(first), total
}

func writeBytes(ctx context.Context) (map[int32]uint64, int) {
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, 0
	}
	m := make(map[int32]uint64, len(ps))
	for _, p := range ps {
		if io, err := p.IOCountersWithContext(ctx); err == nil {
			m[p.Pid] = io.WriteBytes
		}
	}
	return m, len(ps)
}

func describeRise(mount string, from, to float64, ws []diskWriter, readable, total int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "디스크 %s 사용률이 %d분 사이 %.1f%% → %.1f%% (+%.1f%%p) 급증. 쓰기 상위: ",
		mount, diskWatchSamples, from, to, to-from)
	if len(ws) == 0 {
		b.WriteString("없음")
	}
	for i, w := range ws {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s(pid %d", w.name, w.pid)
		if w.user != "" {
			b.WriteString(", " + w.user)
		}
		if w.cwd != "" {
			b.WriteString(", " + w.cwd)
		}
		fmt.Fprintf(&b, ") %.1fMB/s", w.bytesPerSec/1e6)
	}
	if readable < total/2 {
		fmt.Fprintf(&b, " — 프로세스 %d개 중 %d개만 확인 (에이전트에 다른 사용자 프로세스를 읽을 권한 없음)", total, readable)
	}
	return b.String()
}
