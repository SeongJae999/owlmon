package loginsight

import (
	"strings"
	"testing"
)

func TestTemplatize_Timestamp(t *testing.T) {
	cases := []string{
		"2026-05-10 03:21:44 error",
		"2026-05-10T03:21:44Z error",
		"2026-05-10T03:21:44.123+09:00 error",
		"2026-05-10T03:21:44,123 error",
	}
	for _, c := range cases {
		got := Templatize(c)
		if !strings.Contains(got, "<TS>") {
			t.Errorf("expected <TS> in input %q, got %q", c, got)
		}
	}
}

func TestTemplatize_IP(t *testing.T) {
	cases := []string{
		"connection from 192.168.0.30:5432 refused",
		"connection from 10.0.0.5 refused",
	}
	for _, c := range cases {
		got := Templatize(c)
		if !strings.Contains(got, "<IP>") {
			t.Errorf("expected <IP> in %q, got %q", c, got)
		}
	}
}

func TestTemplatize_UUID(t *testing.T) {
	got := Templatize("request 550e8400-e29b-41d4-a716-446655440000 failed")
	if !strings.Contains(got, "<UUID>") {
		t.Errorf("expected <UUID>, got %q", got)
	}
}

func TestTemplatize_Path(t *testing.T) {
	got := Templatize("cannot read /var/log/app/error.log")
	if !strings.Contains(got, "<PATH>") {
		t.Errorf("expected <PATH>, got %q", got)
	}
}

func TestTemplatize_Quoted(t *testing.T) {
	got := Templatize(`user "alice" login failed`)
	if !strings.Contains(got, "<STR>") {
		t.Errorf("expected <STR>, got %q", got)
	}
}

func TestTemplatize_Hex(t *testing.T) {
	got := Templatize("segfault at 0xDEADBEEF in proc")
	if !strings.Contains(got, "<HEX>") {
		t.Errorf("expected <HEX>, got %q", got)
	}
}

func TestTemplatize_Num(t *testing.T) {
	got := Templatize("retry attempt 42 of 100")
	if !strings.Contains(got, "<N>") {
		t.Errorf("expected <N>, got %q", got)
	}
}

func TestTemplatize_NormalizeWhitespace(t *testing.T) {
	got := Templatize("  too    many    spaces  ")
	if got != "too many spaces" {
		t.Errorf("expected normalized whitespace, got %q", got)
	}
}

func TestTemplatize_StablePattern(t *testing.T) {
	// 핵심: 가변 부분만 다른 두 로그가 같은 템플릿을 산출해야 한다.
	a := Templatize("2026-05-10 03:21:44 connection from 192.168.0.10 refused")
	b := Templatize("2026-05-10 03:22:01 connection from 10.0.0.5 refused")
	if a != b {
		t.Errorf("same pattern should yield same template:\n  a=%q\n  b=%q", a, b)
	}
}

func TestHash_Stable(t *testing.T) {
	h1 := Hash("connection refused <IP>")
	h2 := Hash("connection refused <IP>")
	if h1 != h2 {
		t.Errorf("hash not stable: %s vs %s", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("expected 16-char hash, got %d (%q)", len(h1), h1)
	}
}

func TestHash_DifferentInputs(t *testing.T) {
	if Hash("foo") == Hash("bar") {
		t.Error("different inputs should produce different hashes")
	}
}

func TestCluster_GroupsSamePattern(t *testing.T) {
	lines := []LogLine{
		{ID: 1, HostName: "web1", Message: "2026-05-10 03:21:44 connection from 192.168.0.10 refused"},
		{ID: 2, HostName: "web1", Message: "2026-05-10 03:22:01 connection from 10.0.0.5 refused"},
		{ID: 3, HostName: "web1", Message: "2026-05-10 03:22:30 connection from 172.16.1.20 refused"},
		{ID: 4, HostName: "web1", Message: "2026-05-10 03:23:00 disk full at /var/log/app"},
	}
	groups := Cluster(lines, 3)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (refused + disk full), got %d", len(groups))
	}
	var refusedGroup *LogGroup
	for i := range groups {
		if strings.Contains(groups[i].TemplateExample, "refused") {
			refusedGroup = &groups[i]
		}
	}
	if refusedGroup == nil {
		t.Fatal("refused group not found")
	}
	if refusedGroup.Count != 3 {
		t.Errorf("expected count=3 for refused, got %d", refusedGroup.Count)
	}
	if len(refusedGroup.SampleLines) != 3 {
		t.Errorf("expected 3 samples, got %d", len(refusedGroup.SampleLines))
	}
	if len(refusedGroup.SampleLogIDs) != 3 {
		t.Errorf("expected 3 sample IDs, got %d", len(refusedGroup.SampleLogIDs))
	}
}

func TestCluster_DifferentHostsSeparate(t *testing.T) {
	// 같은 패턴이라도 호스트가 다르면 별도 그룹.
	lines := []LogLine{
		{ID: 1, HostName: "web1", Message: "disk full at /var/log/app"},
		{ID: 2, HostName: "web2", Message: "disk full at /var/log/app"},
	}
	groups := Cluster(lines, 3)
	if len(groups) != 2 {
		t.Errorf("expected 2 groups (different hosts), got %d", len(groups))
	}
}

func TestCluster_SampleLimitRespected(t *testing.T) {
	// 같은 패턴 100건 → 샘플은 maxSamples만큼만, Count는 전체.
	lines := make([]LogLine, 100)
	for i := range lines {
		lines[i] = LogLine{ID: int64(i), HostName: "web1", Message: "connection refused"}
	}
	groups := Cluster(lines, 3)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Count != 100 {
		t.Errorf("expected count=100, got %d", groups[0].Count)
	}
	if len(groups[0].SampleLines) != 3 {
		t.Errorf("expected 3 samples, got %d", len(groups[0].SampleLines))
	}
}

func TestCluster_DeterministicOrder(t *testing.T) {
	// 두 번 호출해도 같은 순서가 나와야 한다 (입력 등장 순서 기준).
	lines := []LogLine{
		{ID: 1, HostName: "web1", Message: "alpha 1"},
		{ID: 2, HostName: "web1", Message: "beta 2"},
		{ID: 3, HostName: "web1", Message: "alpha 3"},
	}
	g1 := Cluster(lines, 3)
	g2 := Cluster(lines, 3)
	if len(g1) != len(g2) {
		t.Fatalf("group count differs: %d vs %d", len(g1), len(g2))
	}
	for i := range g1 {
		if g1[i].TemplateHash != g2[i].TemplateHash {
			t.Errorf("group order differs at %d: %s vs %s",
				i, g1[i].TemplateHash, g2[i].TemplateHash)
		}
	}
}

func TestBuildUserMsg(t *testing.T) {
	g := LogGroup{
		TemplateHash:    "abc12345",
		TemplateExample: "connection refused <IP>",
		HostName:        "web1",
		Count:           5,
		SampleLines:     []string{"line1", "line2"},
	}
	msg := BuildUserMsg(g)
	for _, want := range []string{"Host: web1", "5 times", "[1] line1", "[2] line2", "connection refused"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in user msg:\n%s", want, msg)
		}
	}
}

func TestTemplatize_AuthUser_SSHBruteForce(t *testing.T) {
	// 핵심: 계정명만 다른 brute-force 로그가 같은 템플릿을 산출해야 한다 (E2E에서 갈라졌던 케이스).
	a := Templatize("Failed password for root from 203.0.113.9 port 44512 ssh2")
	b := Templatize("Failed password for admin from 203.0.113.9 port 44513 ssh2")
	if a != b {
		t.Errorf("same brute-force pattern should yield same template:\n  a=%q\n  b=%q", a, b)
	}
	if !strings.Contains(a, "<USER>") {
		t.Errorf("expected <USER> in template, got %q", a)
	}
}

func TestTemplatize_AuthUser_Variants(t *testing.T) {
	cases := []string{
		"Failed password for invalid user oracle from 10.0.0.1 port 22 ssh2",
		"Invalid user postgres from 10.0.0.2 port 51234",
		"Accepted publickey for deploy from 10.0.0.3 port 22 ssh2",
		"pam_unix(sshd:session): session opened for user root by (uid=0)",
		"pam_unix(sshd:auth): authentication failure; logname= uid=0 euid=0 user=guest",
	}
	for _, c := range cases {
		got := Templatize(c)
		if !strings.Contains(got, "<USER>") {
			t.Errorf("expected <USER> in %q, got %q", c, got)
		}
	}
}

func TestTemplatize_AuthUser_DigitUsername(t *testing.T) {
	// 숫자 포함 계정명(user123)이 <N> 치환으로 깨지기 전에 <USER>로 묶여야 한다.
	a := Templatize("Failed password for user123 from 10.0.0.1 port 22 ssh2")
	b := Templatize("Failed password for backup7 from 10.0.0.1 port 22 ssh2")
	if a != b {
		t.Errorf("digit usernames should still cluster:\n  a=%q\n  b=%q", a, b)
	}
}

func TestTemplatize_AuthUser_NoOvermask(t *testing.T) {
	// auth 관용구가 아닌 일반 문장의 "for ... from"은 마스킹하면 안 된다.
	cases := []string{
		"waiting for connection from peer",
		"retry scheduled for tomorrow",
		"checking for updates from registry",
	}
	for _, c := range cases {
		got := Templatize(c)
		if strings.Contains(got, "<USER>") {
			t.Errorf("over-masked non-auth line %q → %q", c, got)
		}
	}
}
