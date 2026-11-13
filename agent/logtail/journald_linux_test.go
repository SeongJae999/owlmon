//go:build linux

package logtail

import "testing"

func TestParseJournaldLine_StdoutLevelFromMessage(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{"PRIORITY":"6","MESSAGE":"12-Nov-2026 10:15:22.123 SEVERE [main] Context startup failed"}`, "ERROR"},
		{`{"PRIORITY":"6","MESSAGE":"Started worker"}`, "INFO"},
		{`{"PRIORITY":"3","MESSAGE":"INFO but priority says err"}`, "ERROR"},
	}
	for _, c := range cases {
		e, ok := parseJournaldLine([]byte(c.raw), "h", "journald")
		if !ok || e.Level != c.want {
			t.Errorf("%s → %q, want %q", c.raw, e.Level, c.want)
		}
	}
}
