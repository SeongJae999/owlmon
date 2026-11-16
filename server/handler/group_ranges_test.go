package handler

import (
	"slices"
	"testing"
)

func TestNormalizeRanges(t *testing.T) {
	got, err := normalizeRanges([]string{" 192.168.0.7/24 ", "10.0.0.5~10.0.0.1", "10.0.0.1 - 10.0.0.9", "172.16.0.3", "", "172.16.0.3", "1.1.1.1-1.1.1.1"})
	if err == nil {
		t.Fatalf("거꾸로 된 범위를 받아들임: %v", got)
	}
	got, err = normalizeRanges([]string{" 192.168.0.7/24 ", "10.0.0.1 - 10.0.0.9", "10.0.0.1~10.0.0.9", "172.16.0.3", "", "172.16.0.3", "1.1.1.1-1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.168.0.0/24", "10.0.0.1-10.0.0.9", "172.16.0.3", "1.1.1.1"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, bad := range []string{"192.168.0.0/33", "fe80::1", "fe80::/64", "300.1.1.1", "abc", "10.0.0.1-", "1.2.3.4/24/1"} {
		if _, err := normalizeRanges([]string{bad}); err == nil {
			t.Errorf("잘못된 값 통과: %q", bad)
		}
	}
	if r, _ := normalizeRanges(nil); r == nil {
		t.Fatal("nil 대신 빈 배열이어야 함")
	}
}
