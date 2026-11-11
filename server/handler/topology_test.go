package handler

import (
	"testing"

	"github.com/seongJae/owlmon/server/db"
)

func TestValidateLayout(t *testing.T) {
	zones := []string{"인터넷", "내부망"}
	dev := func(name, kind, zone, parent string) db.TopologyDevice {
		return db.TopologyDevice{Name: name, Kind: kind, Zone: zone, Parent: parent}
	}
	base := []db.TopologyDevice{dev("인터넷", "internet", "인터넷", ""), dev("코어", "network", "내부망", "인터넷")}
	cases := []struct {
		name  string
		l     db.TopologyLayout
		valid bool
	}{
		{"빈 레이아웃", db.TopologyLayout{}, true},
		{"정상", db.TopologyLayout{Zones: zones, Devices: base}, true},
		{"이중화 짝", db.TopologyLayout{Zones: zones, Devices: append(base,
			db.TopologyDevice{Name: "코어B", Kind: "network", Zone: "내부망", Parent: "인터넷", Pair: "코어"})}, true},
		{"서로 짝으로 적음", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{
			{Name: "a", Kind: "server", Zone: "내부망", Pair: "b"}, {Name: "b", Kind: "server", Zone: "내부망", Pair: "a"}}}, true},
		{"짝이 사슬로 이어짐", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{
			{Name: "a", Kind: "server", Zone: "내부망", Pair: "b"}, {Name: "b", Kind: "server", Zone: "내부망", Pair: "c"}, {Name: "c", Kind: "server", Zone: "내부망"}}}, false},
		{"구역 중복", db.TopologyLayout{Zones: []string{"a", "a"}}, false},
		{"없는 구역", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{dev("a", "server", "DMZ", "")}}, false},
		{"모르는 종류", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{dev("a", "printer", "내부망", "")}}, false},
		{"이름 중복", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{dev("a", "server", "내부망", ""), dev("a", "server", "내부망", "")}}, false},
		{"없는 상위", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{dev("a", "server", "내부망", "b")}}, false},
		{"자기 자신이 짝", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{{Name: "a", Kind: "server", Zone: "내부망", Pair: "a"}}}, false},
		{"순환", db.TopologyLayout{Zones: zones, Devices: []db.TopologyDevice{dev("a", "server", "내부망", "b"), dev("b", "server", "내부망", "a")}}, false},
	}
	for _, c := range cases {
		if got := validateLayout(c.l) == ""; got != c.valid {
			t.Errorf("%s: valid=%v, want %v (%s)", c.name, got, c.valid, validateLayout(c.l))
		}
	}
}
