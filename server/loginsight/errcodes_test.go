package loginsight

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/seongJae/owlmon/server/masking"
)

// 해설이 틀리거나 심각도를 정하면 모든 고객에게 같은 오답이 퍼진다 — 사전에 넣는 문구의 규칙을 기계로 지킨다
func TestErrCodeEntries(t *testing.T) {
	phraseLines := map[string]string{
		"MSSQL-17573": "CHECKDB for database 'master' finished without errors on 2026-11-01 02:00:01.000 (local time).",
		"MSSQL-17111": `Logging SQL Server messages in file 'C:\Program Files\Microsoft SQL Server\MSSQL\Log\ERRORLOG'.`,
	}
	handoffs := map[string]bool{"": true, "DB 유지보수 업체": true, "프로그램 개발사": true, "네트워크 담당": true,
		"서버·스토리지 담당": true, "서버·도메인 담당": true, "보안 담당": true}
	gradeWords := []string{"심각", "치명", "긴급", "위험", "무시", "안전"}
	actionWords := []string{"재시작", "재기동하", "재설치", "패치", "삭제", "fsck", "교체", "축소"}
	for key, c := range errCodes {
		if !strings.HasPrefix(c.source, "https://") {
			t.Errorf("%s: 출처 URL 없음", key)
		}
		if n := utf8.RuneCountInString(c.meaning); n == 0 || n > 80 {
			t.Errorf("%s: 뜻 %d자 (1~80)", key, n)
		}
		if n := utf8.RuneCountInString(c.caution); n == 0 || n > 40 {
			t.Errorf("%s: 오해 주의 %d자 (1~40)", key, n)
		}
		if !handoffs[c.handoff] {
			t.Errorf("%s: 넘길 곳 %q 은 정해진 값이 아님", key, c.handoff)
		}
		all := c.meaning + " " + c.caution + " " + c.handoff
		for _, w := range gradeWords {
			if strings.Contains(all, w) {
				t.Errorf("%s: 심각도 낱말 %q", key, w)
			}
		}
		for _, w := range actionWords {
			if strings.Contains(c.meaning, w) {
				t.Errorf("%s: 뜻에 실행 조치 %q", key, w)
			}
		}
		line, ok := phraseLines[key]
		if !ok {
			num := key[strings.IndexByte(key, '-')+1:]
			if strings.HasPrefix(key, "MSSQL-") {
				line = "2026-11-14 10:00:00.00 spid52      Error: " + num + ", Severity: 16, State: 1."
			} else {
				line = "TBR-" + num + ": message"
			}
		}
		if h := findCodes(line); len(h) != 1 || h[0].key != key {
			t.Errorf("%s: 예시 줄에서 다시 찾지 못함 %+v", key, h)
		}
	}
	if strings.Count(SystemPromptLogCluster, ErrCodeRules) != 1 {
		t.Fatal("해설 없는 분석은 시스템 프롬프트에서 ErrCodeRules를 한 번 떼어 낸다")
	}
}

func TestTemplatize_KeepsDictCodes(t *testing.T) {
	a := Templatize("2026-11-14 10:00:00.12 spid52      Error: 9002, Severity: 17, State: 2.")
	b := Templatize("2026-11-14 10:00:00.12 spid52      Error: 18456, Severity: 14, State: 8.")
	if a == b || !strings.Contains(a, "Error: 9002, Severity: <N>, State: <N>.") {
		t.Fatalf("사전 코드는 남고 나머지 숫자만 지워져야 함:\n%s\n%s", a, b)
	}
	for in, want := range map[string]string{
		"TBR-2131: Generic I/O error.":            "TBR-2131: Generic I/O error.",
		"TBR-2137: Generic I/O error.":            "TBR-<N>: Generic I/O error.", // 사전 밖 번호는 지금처럼 지운다
		"INC-12345 Error: 404":                    "INC-<N> Error: <N>",
		"[ERROR] ec=ERROR_TBR_IO(-2131) tid 4417": "[ERROR] ec=ERROR_TBR_IO(-2131) tid <N>",
		"ERROR_SECU_LOGIN_FAILED(-17001) [ Login failed: invalid user name or password.]": "ERROR_SECU_LOGIN_FAILED(-17001) [ Login failed: invalid user <USER> or password.]",
		"java.sql.SQLException: JDBC-12033:Lock fail in NOWAIT":                           "java.sql.SQLException: JDBC-12033:Lock fail in NOWAIT",
		"java.sql.SQLException: JDBC-90603:Unexpected end of stream":                      "java.sql.SQLException: JDBC-<N>:Unexpected end of stream",
	} {
		if got := Templatize(in); got != want {
			t.Errorf("Templatize(%q) = %q, want %q", in, got, want)
		}
	}
	t.Setenv("OWLMON_LOGINSIGHT_ERRCODE_DICT", "off")
	if got := Templatize("TBR-2131: Generic I/O error."); got != "TBR-<N>: Generic I/O error." {
		t.Errorf("사전을 끄면 이전처럼 번호를 지워야 함: %q", got)
	}
}

func TestBuildUserMsg_ErrCodeNotes(t *testing.T) {
	g := LogGroup{HostName: "db1", Count: 3, TemplateExample: "x", PriorAnalyses: 2,
		SampleLines: []string{
			"TBR-2131: Generic I/O error.",
			"TBR-99999: unknown and TBR-2131 again",
			"2026-11-14 10:00:00.12 spid52 Error: 9002, Severity: 17, State: 2.",
		}}
	msg := BuildUserMsg(g)
	notes := errCodeNotes(g.SampleLines)
	if strings.Count(notes, "\n- ") != 2 || !strings.Contains(notes, "- TBR-2131: ") || strings.Index(notes, "TBR-2131") > strings.Index(notes, "MSSQL-9002") {
		t.Fatalf("등장 순서대로 중복 없이 사전 코드만:\n%s", notes)
	}
	samples, block, hist := strings.Index(msg, "Sample log lines"), strings.Index(msg, "[오류 코드 해설]"), strings.Index(msg, "[패턴 이력]")
	if !(samples < block && block < hist) {
		t.Fatalf("해설 블록은 샘플 뒤, 패턴 이력 앞:\n%s", msg)
	}
	if !strings.Contains(masking.Mask(msg, masking.DefaultOptions()), notes) {
		t.Fatal("해설 문구가 개인정보 마스킹에 걸리면 안 됨")
	}

	many := "Error: 823, Severity: 24 Error: 824, Severity: 24 Error: 701, Severity: 17 Error: 1204, Severity: 19 Error: 912, Severity: 21 Error: 3417, Severity: 21"
	if n := strings.Count(errCodeNotes([]string{many}), "\n- "); n != maxErrCodeNotes {
		t.Errorf("최대 %d개, got %d", maxErrCodeNotes, n)
	}
	if errCodeNotes([]string{"TBR-99999: unknown"}) != "" {
		t.Error("사전에 없는 코드만 있으면 블록을 만들지 않음")
	}

	t.Setenv("OWLMON_LOGINSIGHT_ERRCODE_OFF", "TBR-2131, MSSQL-18456")
	if n := errCodeNotes(g.SampleLines); strings.Contains(n, "TBR-2131") || !strings.Contains(n, "MSSQL-9002") {
		t.Errorf("코드별로 끈 해설만 빠져야 함:\n%s", n)
	}
	t.Setenv("OWLMON_LOGINSIGHT_ERRCODE_DICT", "off")
	if strings.Contains(BuildUserMsg(g), "[오류 코드 해설]") {
		t.Error("사전 전체를 끄면 블록이 없어야 함")
	}
}

// 해설을 끄면 옛 해설로 만든 분석은 버리되(캐시 판정), [패턴 이력]·화면 카드 키인 TemplateHash는 그대로
func TestErrCodeOff_InvalidatesCacheNotHistory(t *testing.T) {
	lines := []LogLine{{ID: 1, HostName: "h", Message: "TBR-2131: Generic I/O error."}, {ID: 2, HostName: "h", Message: "disk full on /var/lib"}}
	before := Cluster(lines, 3)
	sig := errCodeStateSig()
	t.Setenv("OWLMON_LOGINSIGHT_ERRCODE_OFF", "TBR-2131")
	after := Cluster(lines, 3)
	if before[0].TemplateHash != after[0].TemplateHash {
		t.Error("해설을 꺼도 TemplateHash는 그대로여야 이력이 이어짐")
	}
	if errCodeCacheOK(sig) {
		t.Error("해설을 끄면 해설을 붙여 만든 분석은 재사용하지 않아야 함")
	}
	if !errCodeCacheOK("") {
		t.Error("해설 없이 만든 분석은 그대로 재사용")
	}
	offSig := errCodeStateSig()
	t.Setenv("OWLMON_LOGINSIGHT_ERRCODE_OFF", " TBR-99999, TBR-2131 ")
	if errCodeStateSig() != offSig {
		t.Error("OFF 값의 공백·순서·사전에 없는 코드만 바뀌면 서명이 같아야 함")
	}
	t.Setenv("OWLMON_LOGINSIGHT_ERRCODE_DICT", "off")
	if errCodeCacheOK(sig) {
		t.Error("사전 전체를 꺼도 해설을 붙여 만든 분석은 재사용하지 않아야 함")
	}
}

// 따옴표·계정명·경로는 외부에서 넣을 수 있는 자리 — 거기 든 코드로 묶음이 쪼개지거나 해설이 붙으면 안 된다
func TestErrCodes_IgnoreAttackerPositions(t *testing.T) {
	cases := []struct{ withCode, plain string }{
		{`2026/11/14 09:12:44 [error] 1340#0: *50211 open() "/usr/share/nginx/html/kb/Error: 824, Severity: 24, State: 2." failed (2: No such file or directory), client: 10.20.1.57, server: _, request: "GET /kb/x HTTP/1.1"`,
			`2026/11/14 09:12:44 [error] 1340#0: *50211 open() "/usr/share/nginx/html/kb/x" failed (2: No such file or directory), client: 10.20.1.57, server: _, request: "GET /kb/x HTTP/1.1"`},
		{`Invalid user TBR-1063 from 203.0.113.5 port 51234`, `Invalid user oracle from 203.0.113.5 port 51234`},
		{`Failed password for invalid user TBR-1063-TBR-2131 from 203.0.113.5 port 22 ssh2`, `Failed password for invalid user admin from 203.0.113.5 port 22 ssh2`},
		{`WARN Resolved [MethodArgumentTypeMismatchException: For input string: "TBR-1063"]`, `WARN Resolved [MethodArgumentTypeMismatchException: For input string: "abc"]`},
		{`WARN No mapping for GET /TBR-1063`, `WARN No mapping for GET /TBR-9`},
		{`WARN No mapping for GET /board/view/TBR-2131`, `WARN No mapping for GET /board/view/x`},
		{`WARN No mapping for GET /x-TBR-1063`, `WARN No mapping for GET /x-TBR-9`},
		{`WARN No mapping for GET /x.TBR-1063`, `WARN No mapping for GET /x.TBR-9`},
		{`Invalid user a,TBR-1063 from 203.0.113.5 port 51234`, `Invalid user oracle from 203.0.113.5 port 51234`},
		{`Failed password for invalid user a;TBR-12003 from 203.0.113.5 port 22 ssh2`, `Failed password for invalid user admin from 203.0.113.5 port 22 ssh2`},
		{`pam_unix(sshd:auth): authentication failure; rhost=203.0.113.5 user=a,TBR-1063`, `pam_unix(sshd:auth): authentication failure; rhost=203.0.113.5 user=root,TBR-9`},
		{`2026/11/14 09:12:44 [error] 1340#0: open() /var/www/kb/Error: 824, Severity: 24 failed`, `2026/11/14 09:12:44 [error] 1340#0: open() /var/www/kb/Error: 9, Severity: 1 failed`},
		{`WARN rejected file C:\upload\TBR-1063.txt`, `WARN rejected file C:\upload\TBR-9.txt`},
		{`Invalid user x TBR-1063 from 203.0.113.5 port 51234`, `Invalid user oracle from 203.0.113.5 port 51234`},
		{`Failed password for invalid user x TBR-12003 from 203.0.113.5 port 22 ssh2`, `Failed password for invalid user admin from 203.0.113.5 port 22 ssh2`},
		{`Login failed for user 'x Error: 824, Severity: 24'. Reason: Could not find a login matching the name provided. [CLIENT: 203.0.113.5]`,
			`Login failed for user 'sa'. Reason: Could not find a login matching the name provided. [CLIENT: 203.0.113.5]`},
		{`WARN No mapping for GET /board/view,TBR-2131`, `WARN No mapping for GET /board/view,TBR-9`},
		{`WARN No mapping for GET /board/view:TBR-1063`, `WARN No mapping for GET /board/view:TBR-9`},
	}
	// 계정명 규칙이 key=value 머리말의 다른 필드까지 지우면 다른 DB의 오류가 한 묶음이 된다
	if Templatize(`[1234]: user=app,db=erp,client=10.0.0.5 ERROR: relation "x" does not exist`) ==
		Templatize(`[1234]: user=app,db=hr,client=10.0.0.5 ERROR: relation "x" does not exist`) {
		t.Error("user= 뒤의 다른 필드는 남아야 함")
	}
	for _, c := range cases {
		if n := errCodeNotes([]string{c.withCode}); n != "" {
			t.Errorf("해설이 붙으면 안 됨: %q\n%s", c.withCode, n)
		}
		if a, b := Templatize(c.withCode), Templatize(c.plain); a != b {
			t.Errorf("코드를 넣어도 같은 묶음이어야 함:\n%s\n%s", a, b)
		}
	}
	if Templatize("x hgmj y") != "x hgmj y" {
		t.Error("원문의 표식 문자로 코드 번호를 만들어 내면 안 됨")
	}
	for _, l := range []string{
		"2026-11-01 02:00:01.00 spid9s CHECKDB for database 'master' finished without errors on 2026-11-01 02:00:01.000 (local time).",
		"java.sql.SQLException: JDBC-12033:Lock fail in NOWAIT",
		"[11-13T15:22:08.441902] [FRM-142] [I] THROW. ec=ERROR_TOO_MANY_OPEN_CURSORS_USING_SAME_OBJECT(-12098) [ x ] (csr_id:9021)",
	} {
		if errCodeNotes([]string{l}) == "" {
			t.Errorf("구조 자리의 코드는 해설이 붙어야 함: %q", l)
		}
	}
}
