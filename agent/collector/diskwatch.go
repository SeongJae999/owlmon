package collector

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
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
				ws, note := topDiskWriters(ctx, diskWatchMeasure)
				report(describeRise(p.Mountpoint, from, u.UsedPercent, ws, note))
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
	label       string // "unzip(pid 1234, will, /home/will/dev)" 또는 "mysql.service"
	bytesPerSec float64
}

// topDiskWriters는 d 동안 디스크에 가장 많이 쓴 프로세스를 잰다.
// 에이전트는 전용 계정으로 돌아 다른 계정 프로세스의 쓰기량(/proc/PID/io, 소유자만 읽기)을 못 읽는다.
// 절반 넘게 못 읽으면 누구나 읽을 수 있는 cgroup io.stat으로 서비스·컨테이너·사용자 세션 단위를 대신 보여 준다.
func topDiskWriters(ctx context.Context, d time.Duration) (ws []diskWriter, note string) {
	first, total := writeBytes(ctx)
	root, file := cgroupRoot, "io.stat"
	cgFirst := cgroupWriteBytes(root, file)
	if len(cgFirst) == 0 { // cgroup v1(CentOS 7·8 등)은 blkio 아래에 같은 값을 둔다
		root, file = filepath.Join(cgroupRoot, "blkio"), "blkio.throttle.io_service_bytes"
		cgFirst = cgroupWriteBytes(root, file)
	}
	select {
	case <-ctx.Done():
		return nil, ""
	case <-time.After(d):
	}
	if len(first) >= total/2 {
		second, _ := writeBytes(ctx)
		for pid, b := range second {
			if a, ok := first[pid]; ok && b > a {
				ws = append(ws, diskWriter{label: processLabel(ctx, pid), bytesPerSec: float64(b-a) / d.Seconds()})
			}
		}
		return top(ws), ""
	}
	if len(cgFirst) == 0 {
		return nil, fmt.Sprintf("프로세스 %d개 중 %d개만 확인 (다른 계정 프로세스를 읽을 수 없고 cgroup 쓰기 통계도 없음)", total, len(first))
	}
	for path, b := range cgroupWriteBytes(root, file) {
		if a, ok := cgFirst[path]; ok && b > a {
			ws = append(ws, diskWriter{label: cgroupLabel(root, path), bytesPerSec: float64(b-a) / d.Seconds()})
		}
	}
	return top(ws), ""
}

func top(ws []diskWriter) []diskWriter {
	sort.Slice(ws, func(i, j int) bool { return ws[i].bytesPerSec > ws[j].bytesPerSec })
	if len(ws) > topWriters {
		ws = ws[:topWriters]
	}
	return ws
}

func processLabel(ctx context.Context, pid int32) string {
	p := &process.Process{Pid: pid}
	name, _ := p.NameWithContext(ctx)
	l := fmt.Sprintf("%s(pid %d", name, pid)
	if u, _ := p.UsernameWithContext(ctx); u != "" {
		l += ", " + u
	}
	if cwd, _ := p.CwdWithContext(ctx); cwd != "" {
		l += ", " + cwd
	}
	return l + ")"
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

const cgroupRoot = "/sys/fs/cgroup"

// cgroupWriteBytes는 cgroup별 누적 쓰기 바이트를 읽는다. 최상위(system.slice 등) 아래에 io.stat이 있으면
// 그 하위(서비스·컨테이너)를, 없으면(user.slice는 사용자별로 안 나뉨) 최상위 자체를 쓴다. 키는 root 기준 경로.
// v2 io.stat("8:0 rbytes=1 wbytes=2 ...")과 v1 blkio("8:0 Write 2") 둘 다 장치별 줄을 더한다.
func cgroupWriteBytes(root, file string) map[string]uint64 {
	m := map[string]uint64{}
	tops, _ := filepath.Glob(filepath.Join(root, "*", file))
	for _, t := range tops {
		files, _ := filepath.Glob(filepath.Join(filepath.Dir(t), "*", file))
		if len(files) == 0 {
			files = []string{t}
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var sum uint64
			fields := strings.Fields(string(data))
			for i, field := range fields {
				v, ok := strings.CutPrefix(field, "wbytes=")
				if !ok && i > 0 && fields[i-1] == "Write" {
					v, ok = field, true
				}
				if ok {
					n, _ := strconv.ParseUint(v, 10, 64)
					sum += n
				}
			}
			rel, _ := filepath.Rel(root, filepath.Dir(f))
			m[rel] = sum
		}
	}
	return m
}

// cgroupLabel은 cgroup 경로를 관제사가 알아볼 이름으로 바꾼다.
func cgroupLabel(root, path string) string {
	base := filepath.Base(path)
	if id, ok := strings.CutPrefix(strings.TrimSuffix(base, ".scope"), "docker-"); ok || filepath.Base(filepath.Dir(path)) == "docker" {
		if !ok {
			id = base
		}
		return "Docker 컨테이너 " + id[:min(12, len(id))]
	}
	if path != "user.slice" {
		return base
	}
	var names []string
	dirs, _ := filepath.Glob(filepath.Join(root, "user.slice", "user-*.slice"))
	for _, d := range dirs {
		uid := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(d), "user-"), ".slice")
		if u, err := user.LookupId(uid); err == nil {
			uid = u.Username
		}
		names = append(names, uid)
	}
	if len(names) == 0 { // v1 blkio는 사용자별로 안 나뉜다
		return "사용자 로그인 세션"
	}
	return "사용자 로그인 세션(" + strings.Join(names, ", ") + ")"
}

func describeRise(mount string, from, to float64, ws []diskWriter, note string) string {
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
		fmt.Fprintf(&b, "%s %.1fMB/s", w.label, w.bytesPerSec/1e6)
	}
	if note != "" {
		b.WriteString(" — " + note)
	}
	return b.String()
}
