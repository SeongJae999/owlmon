package handler

import (
	"testing"

	"github.com/seongJae/owlmon/server/synthetic"
)

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

func TestFindDuplicateURL(t *testing.T) {
	list := []synthetic.Monitor{{ID: 2, Name: "영동알리미", URL: "https://alrimi.yd21.go.kr/"}, {ID: 25, Name: "산단", URL: "https://disp-mobile.willkomo.com/"}}
	cases := []struct {
		name string
		m    synthetic.Monitor
		want bool
	}{
		{"추가: 같은 주소", synthetic.Monitor{URL: "https://alrimi.yd21.go.kr"}, true},
		{"추가: 새 주소", synthetic.Monitor{URL: "https://new.kr/"}, false},
		{"수정: 자기 주소 그대로", synthetic.Monitor{ID: 2, URL: "https://alrimi.yd21.go.kr/"}, false},
		{"수정: 다른 등록의 주소로 바꿈", synthetic.Monitor{ID: 25, URL: "https://ALRIMI.yd21.go.kr/"}, true},
	}
	for _, c := range cases {
		if _, got := findDuplicateURL(list, c.m); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
