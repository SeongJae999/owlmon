package handler

import "testing"

func TestSameMonitorURL(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://alrimi.yd21.go.kr/", "https://alrimi.yd21.go.kr/", true},
		{"https://alrimi.yd21.go.kr/", "https://ALRIMI.yd21.go.kr", true},
		{" https://a.kr/ ", "https://a.kr", true},
		{"https://a.kr/", "https://b.kr/", false},
		{"https://a.kr/login", "https://a.kr/", false},
	}
	for _, c := range cases {
		if got := sameMonitorURL(c.a, c.b); got != c.want {
			t.Errorf("sameMonitorURL(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
