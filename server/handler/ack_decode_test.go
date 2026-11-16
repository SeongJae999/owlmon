package handler

import (
	"strings"
	"testing"
)

func TestDecodeAcks(t *testing.T) {
	one, ok := decodeAcks(strings.NewReader(`{"host":"a","category":"server_down","severity":"critical"}`))
	if !ok || len(one) != 1 || one[0].Host != "a" {
		t.Fatalf("single: %v %v", one, ok)
	}
	many, ok := decodeAcks(strings.NewReader(` [{"host":"a"},{"host":"b","category":"c"}]`))
	if !ok || len(many) != 2 || many[1].Category != "c" {
		t.Fatalf("array: %v %v", many, ok)
	}
	for _, bad := range []string{``, `[]`, `[{"host":""}]`, `{"host":""}`, `nope`} {
		if _, ok := decodeAcks(strings.NewReader(bad)); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}
