package db

import "testing"

func TestRackFitAndLocationText(t *testing.T) {
	fits := []struct {
		unit, size, height int
		ok                 bool
	}{{1, 1, 42, true}, {41, 2, 42, true}, {42, 2, 42, false}, {0, 1, 42, false}, {5, 0, 42, false}}
	for _, c := range fits {
		if got := rackFit(c.unit, c.size, c.height) == ""; got != c.ok {
			t.Errorf("rackFit(%d,%d,%d) ok=%v, want %v", c.unit, c.size, c.height, got, c.ok)
		}
	}
	locs := []struct {
		loc, rack  string
		unit, size int
		want       string
	}{
		{"본관 2층 서버실", "A랙", 14, 1, "본관 2층 서버실 · A랙 14U"},
		{"", "A랙", 14, 2, "A랙 14~15U"},
		{"본관", "", 0, 1, "본관"},
		{"본관", "A랙", 0, 1, "본관"},
		{"", "", 0, 1, ""},
	}
	for _, c := range locs {
		if got := LocationText(c.loc, c.rack, c.unit, c.size); got != c.want {
			t.Errorf("LocationText(%q,%q,%d,%d) = %q, want %q", c.loc, c.rack, c.unit, c.size, got, c.want)
		}
	}
}
