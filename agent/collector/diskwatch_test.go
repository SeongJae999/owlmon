package collector

import (
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
	line := describeRise("/", 84, 86.1, []diskWriter{{pid: 1234, name: "unzip", user: "will", cwd: "/home/will/dev/fire", bytesPerSec: 120e6}}, 10, 300)
	for _, want := range []string{"디스크 / ", "+2.1%p", "unzip(pid 1234, will, /home/will/dev/fire) 120.0MB/s", "300개 중 10개만"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q 없음: %s", want, line)
		}
	}
	if strings.Contains(describeRise("/", 84, 86, nil, 290, 300), "권한") {
		t.Error("대부분 읽혔는데 권한 부족이라고 씀")
	}
}
