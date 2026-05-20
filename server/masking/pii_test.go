package masking

import (
	"net"
	"testing"
)

func TestMaskRRN(t *testing.T) {
	opts := DefaultOptions()
	cases := []struct {
		in, want string
	}{
		{"홍길동 900101-1234567 로그인", "홍길동 [RRN-MASKED] 로그인"},
		{"RRN: 850515-2345678", "RRN: [RRN-MASKED]"},
		// false positive 방지: 1자 다른 패턴은 통과
		{"버전 900101-12345678", "버전 900101-12345678"}, // 8자리는 RRN 아님
	}
	for _, c := range cases {
		got := Mask(c.in, opts)
		if got != c.want {
			t.Errorf("Mask(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	opts := DefaultOptions()
	cases := []struct {
		in, want string
	}{
		{"user@example.com 로그인", "u***@example.com 로그인"},
		{"abcd@gmail.com", "a***@gmail.com"},
	}
	for _, c := range cases {
		got := Mask(c.in, opts)
		if got != c.want {
			t.Errorf("Mask(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMaskPhone(t *testing.T) {
	opts := DefaultOptions()
	got := Mask("연락처 010-1234-5678 전화", opts)
	want := "연락처 010-****-**** 전화"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMaskCard(t *testing.T) {
	opts := DefaultOptions()
	got := Mask("결제 1234-5678-9012-3456 승인", opts)
	want := "결제 [CARD-MASKED] 승인"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMaskExternalIPOff(t *testing.T) {
	opts := DefaultOptions() // MaskExternalIP: false
	in := "from 8.8.8.8 to 192.168.0.30"
	got := Mask(in, opts)
	if got != in {
		t.Errorf("외부 IP 마스킹이 꺼져있어야 함. got %q", got)
	}
}

func TestMaskExternalIPOn(t *testing.T) {
	opts := DefaultOptions()
	opts.MaskExternalIP = true
	// 사내망(192.168) 유지, 외부(8.8.8.8) 마스킹
	got := Mask("from 8.8.8.8 to 192.168.0.30", opts)
	want := "from 8.8.x.x to 192.168.0.30"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsPrivate(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"8.8.8.8", false},
		{"172.30.1.81", true},
		{"172.32.0.1", false}, // 172.32는 사내망 아님
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip).To4()
		if ip == nil {
			t.Fatalf("ParseIP %q failed", c.ip)
		}
		if got := isPrivate(ip); got != c.want {
			t.Errorf("isPrivate(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}
