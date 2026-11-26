package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskRise(t *testing.T) {
	d := diskRise{hist: map[string][]float64{}}
	for i := 0; i < diskWatchSamples; i++ {
		if _, rose := d.observe("/", 80+float64(i)*0.05); rose {
			t.Fatalf("%d분째: 기준 기간이 차기 전에 보고함", i)
		}
	}
	from, rose := d.observe("/", 82)
	if !rose || from != 80 {
		t.Fatalf("10분 +2%%p인데 보고 안 함 (rose=%v from=%v)", rose, from)
	}
	if _, rose := d.observe("/", 82.5); rose {
		t.Fatal("보고 직후 같은 증가를 다시 보고함")
	}
	slow := diskRise{hist: map[string][]float64{}}
	for i := 0; i <= diskWatchSamples; i++ {
		if _, rose := slow.observe("/", 80+float64(i)*0.05); rose {
			t.Fatal("10분 +0.5 포인트 완만한 증가를 보고함")
		}
	}
}

func TestDescribeRise(t *testing.T) {
	line := describeRise("/", 84, 86.1, []diskWriter{{label: "unzip(pid 1234, will, /home/will/dev/fire)", bytesPerSec: 120e6}}, "")
	for _, want := range []string{"디스크 / ", "+2.1%p", "unzip(pid 1234, will, /home/will/dev/fire) 120.0MB/s"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q 없음: %s", want, line)
		}
	}
	if !strings.Contains(describeRise("/", 84, 86, nil, "권한 없음"), "없음 — 권한 없음") {
		t.Error("확인 못 한 이유가 빠짐")
	}
}

func TestCgroupWriteBytes(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("system.slice/io.stat", "8:0 rbytes=0 wbytes=999 rios=0 wios=0\n")
	write("system.slice/mysql.service/io.stat", "8:0 rbytes=1 wbytes=100 rios=0 wios=0\n8:16 rbytes=0 wbytes=50 rios=0 wios=0\n")
	write("system.slice/docker-0123456789abcdef0123.scope/io.stat", "8:0 wbytes=7\n")
	write("user.slice/io.stat", "8:0 rbytes=0 wbytes=300\n")
	got := cgroupWriteBytes(root)
	want := map[string]uint64{"system.slice/mysql.service": 150, "system.slice/docker-0123456789abcdef0123.scope": 7, "user.slice": 300}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v (하위가 있으면 상위는 빼야 함)", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
	if l := cgroupLabel(root, "system.slice/docker-0123456789abcdef0123.scope"); l != "Docker 컨테이너 0123456789ab" {
		t.Errorf("컨테이너 이름 %q", l)
	}
	if l := cgroupLabel(root, "system.slice/mysql.service"); l != "mysql.service" {
		t.Errorf("서비스 이름 %q", l)
	}
	write("user.slice/user-99999.slice/cgroup.procs", "")
	if l := cgroupLabel(root, "user.slice"); l != "사용자 로그인 세션(99999)" {
		t.Errorf("사용자 세션 이름 %q", l)
	}
}
