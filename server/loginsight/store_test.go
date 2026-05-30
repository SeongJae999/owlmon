package loginsight

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBuildListQuery_NoFilters(t *testing.T) {
	q, args := buildListQuery(Filter{})
	if !strings.Contains(q, "WHERE 1=1") {
		t.Errorf("query missing baseline WHERE: %s", q)
	}
	if !strings.Contains(q, "ORDER BY created_at DESC") {
		t.Errorf("missing ORDER BY: %s", q)
	}
	if !strings.Contains(q, "LIMIT $1") {
		t.Errorf("LIMIT placeholder mismatch: %s", q)
	}
	if len(args) != 1 || args[0] != 50 {
		t.Errorf("default limit should be 50, args=%v", args)
	}
}

func TestBuildListQuery_AllFilters(t *testing.T) {
	from := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	q, args := buildListQuery(Filter{
		HostName: "web1",
		Severity: "high",
		From:     from,
		To:       to,
		Limit:    100,
	})
	for _, want := range []string{
		"host_name = $1",
		"severity = $2",
		"created_at >= $3",
		"created_at <= $4",
		"LIMIT $5",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("missing %q in query:\n%s", want, q)
		}
	}
	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %d: %v", len(args), args)
	}
	if args[0] != "web1" || args[1] != "high" {
		t.Errorf("args[0:2] mismatch: %v", args[:2])
	}
	if args[4] != 100 {
		t.Errorf("limit arg mismatch: %v", args[4])
	}
}

func TestBuildListQuery_PartialFilters(t *testing.T) {
	// 일부만 채운 경우 placeholder 순서 재정렬되는지.
	q, args := buildListQuery(Filter{Severity: "critical", Limit: 25})
	if !strings.Contains(q, "severity = $1") {
		t.Errorf("severity should be $1 when others empty: %s", q)
	}
	if !strings.Contains(q, "LIMIT $2") {
		t.Errorf("LIMIT should be $2: %s", q)
	}
	if strings.Contains(q, "host_name =") {
		t.Errorf("host_name should not appear: %s", q)
	}
	if len(args) != 2 || args[0] != "critical" || args[1] != 25 {
		t.Errorf("args mismatch: %v", args)
	}
}

func TestBuildListQuery_LimitClamp(t *testing.T) {
	cases := []struct {
		name      string
		in, want  int
	}{
		{"zero", 0, 50},
		{"negative", -1, 50},
		{"over_500", 1000, 50},
		{"normal", 100, 100},
		{"max", 500, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, args := buildListQuery(Filter{Limit: c.in})
			last := args[len(args)-1]
			if last != c.want {
				t.Errorf("Limit %d → expected %d, got %v", c.in, c.want, last)
			}
		})
	}
}

func TestNullHelpers(t *testing.T) {
	if v := nullStr(""); v != nil {
		t.Errorf("nullStr empty should be nil, got %v", v)
	}
	if v := nullStr("hello"); v != "hello" {
		t.Errorf("nullStr non-empty: %v", v)
	}
	if v := nullInt(0); v != nil {
		t.Errorf("nullInt zero should be nil, got %v", v)
	}
	if v := nullInt(42); v != 42 {
		t.Errorf("nullInt non-zero: %v", v)
	}
}

func TestNilStore(t *testing.T) {
	ctx := context.Background()
	var s *Store
	if err := s.Save(ctx, &Insight{}); err == nil {
		t.Error("nil store Save should error")
	}
	if _, err := s.List(ctx, Filter{}); err == nil {
		t.Error("nil store List should error")
	}
	if _, err := s.ByTemplate(ctx, "x", time.Hour); err == nil {
		t.Error("nil store ByTemplate should error")
	}
	if err := s.TouchTemplate(ctx, "x"); err == nil {
		t.Error("nil store TouchTemplate should error")
	}
}

func TestNewStore_NilPool(t *testing.T) {
	if NewStore(nil) != nil {
		t.Error("NewStore(nil) should return nil")
	}
}
