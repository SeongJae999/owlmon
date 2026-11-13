package logtail

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/seongJae/owlmon/agent/config"
	"golang.org/x/text/encoding/korean"
	xunicode "golang.org/x/text/encoding/unicode"
)

func TestDetectLevel(t *testing.T) {
	cases := []struct {
		name, line, want string
	}{
		// 미탐 방지
		{"Tomcat SEVERE", "12-Nov-2026 10:15:22.123 SEVERE [main] org.apache.catalina.core.StandardContext.startInternal One or more listeners failed to start.", "ERROR"},
		{"JUL 둘째 줄 SEVERE", "SEVERE: Servlet.service() for servlet [jsp] threw exception", "ERROR"},
		{"한국어 JUL 심각", "심각: 서블릿 [jsp]을(를) 위한 Servlet.service() 호출이 예외를 발생시켰습니다.", "ERROR"},
		{"한국어 Tomcat 경고", "12-Nov-2026 10:15:22.123 경고 [main] org.apache.catalina.loader.WebappClassLoaderBase.clearReferencesThreads 스레드를 중지시키지 못했습니다.", "WARN"},
		{"한국어 robocopy 오류", "2026/11/12 02:00:01 오류 32 (0x00000020) 대상 파일 \\\\filesvr\\backup\\a.txt", "ERROR"},
		{"nginx crit", "2026/11/12 10:15:22 [crit] 1234#1234: *5 SSL_do_handshake() failed while SSL handshaking", "FATAL"},
		{"nginx alert", "2026/11/12 10:15:22 [alert] 1234#1234: 1024 worker_connections are not enough", "FATAL"},
		{"nginx emerg", "2026/11/12 10:15:22 [emerg] 1234#1234: bind() to 0.0.0.0:80 failed (98: Address already in use)", "FATAL"},
		{"Apache core:alert", "[Wed Nov 12 10:15:22.123456 2026] [core:alert] [pid 2312] AH00052: child pid 4321 exit signal Segmentation fault (11)", "FATAL"},
		{"PostgreSQL PANIC", "2026-11-12 10:15:22 KST [1234] PANIC:  could not write to file \"pg_wal/xlogtemp.1234\": No space left on device", "FATAL"},
		{"Tibero 코드만", "TBR-12063: Connection refused by listener.", "ERROR"},
		{"JEUS [0]=SEVERE", "[2026.11.12 10:15:22][0][b056] [container1-21] [SERVER-0242] Unable to get a connection from the connection pool", "ERROR"},
		{"JEUS [1]=WARNING", "[2020.07.12 12:06:03][1] [adminServer-92] [WEB-3347] Connecting to WebtoB[localhost:9900] failed because of a network error", "WARN"},
		{"JEUS [2]=INFO", "[2026.11.12 10:15:22][2] [adminServer-1] [Core-0004] Successfully started the server.", "INFO"},
		{"logback WARN", "10:15:22.123 [main] WARN  c.e.App - connection pool exhausted", "WARN"},

		// eval-set-customer 도달 유지
		{"MSSQL 번호 줄", "2018-09-06 05:51:10.57 spid160 Error: 9002, Severity: 17, State: 2.", "ERROR"},
		{"MSSQL I/O error", "2022-11-02 15:46:42.90 spid51 SQL Server detected a logical consistency-based I/O error: incorrect pageid (expected 1:43686; actual 0:0).", "ERROR"},
		{"Java Error 클래스", "java.lang.OutOfMemoryError: GC overhead limit exceeded", "ERROR"},
		{"Tibero ec=ERROR_", "[11-12T02:14:07.512833] [FRM-87] [I] THROW. ec=ERROR_TX_CANT_ALLOC_EXT(-21004) [ No more extent available in tablespace 'USR'. ]", "ERROR"},
		{"Apache mpm_winnt:warn", "[Thu Apr 19 20:43:33.559350 2012] [mpm_winnt:warn] [pid 3884:tid 1504] (OS 64)The specified network name is no longer available.", "WARN"},
		{"WebtoB [error]", "[08/Jul/2015:17:03:10 +0900] [error] [client 65.65.65.65] TIMEOUT is expired while svc is running", "ERROR"},
		{"robocopy ERROR", "2015/03/10 23:55:35 ERROR 32 (0x00000020) Time-Stamping Destination File \\\\x.x.x.x\\share1\\Dummy_100.txt", "ERROR"},

		// 오탐 방지
		{"errors=0", "2026-11-12 10:15:22 INFO Backup finished: files=120 errors=0", "INFO"},
		{"ErrorPageFilter", "2026-11-12 10:15:22.123  INFO 1234 --- [main] o.s.b.w.servlet.support.ErrorPageFilter : Registered filter", "INFO"},
		{"error.log 경로", "2026-11-12 10:15:22 INFO Logging to /var/log/nginx/error.log", "INFO"},
		{"레벨 없는 error.log", "rotated error.log (size 10485760)", ""},
		{"CHECKDB 0 errors", "2022-09-01 17:36:31.80 spid54 DBCC CHECKDB (ProductionData) executed by DomainName\\UserName found 0 errors and repaired 0 errors.", ""},
		{"URL 경로", "GET /api/error HTTP/1.1 200 512", ""},
		{"DEBUG 줄의 error 필드", "DEBUG response ok error=null", "DEBUG"},
		{"선언 레벨 우선", "WARN retry 1/3 after ERROR response", "WARN"},
		{"본문 alert 단어", "Sending alert mail to admin", ""},
		{"0 warnings", "Build finished with 0 warnings", ""},

		{"레벨 없음", "plain message", ""},
	}
	for _, c := range cases {
		if got := detectLevel(c.line); got != c.want {
			t.Errorf("%s: detectLevel(%q) = %q, want %q", c.name, c.line, got, c.want)
		}
	}
}

func TestNewLineReader_UTF16LE(t *testing.T) {
	enc := func(bom bool, s string) []byte {
		p := xunicode.IgnoreBOM
		if bom {
			p = xunicode.UseBOM
		}
		b, _ := xunicode.UTF16(xunicode.LittleEndian, p).NewEncoder().Bytes([]byte(s))
		return b
	}
	p := t.TempDir() + "/ERRORLOG"
	os.WriteFile(p, enc(true, "2026-11-12 10:00:00.00 Server      시작\r\n"), 0644)
	f, _ := os.Open(p)
	defer f.Close()
	f.Seek(0, io.SeekEnd)

	tl := &Tailer{}
	r := newLineReader(f)
	w, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	w.Write(enc(false, "2026-11-12 10:00:01.00 spid51 Error: 9002, Severity: 17, State: 2.\r\n"))
	w.Close()
	tl.drainNewLines(r, config.TailConfig{}, "")
	if len(tl.buffer) != 1 || tl.buffer[0].Line != "2026-11-12 10:00:01.00 spid51 Error: 9002, Severity: 17, State: 2." || tl.buffer[0].Level != "ERROR" {
		t.Fatalf("Seek 이후 줄: %+v", tl.buffer)
	}

	os.WriteFile(p, enc(true, "2026-11-12 11:00:00.00 spid9 심각한 오류 발생\r\n"), 0644)
	f.Seek(0, io.SeekStart)
	tl.drainNewLines(newLineReader(f), config.TailConfig{}, "")
	if len(tl.buffer) != 2 || tl.buffer[1].Line != "2026-11-12 11:00:00.00 spid9 심각한 오류 발생" {
		t.Fatalf("처음부터 읽은 줄에 BOM이 남거나 깨짐: %+v", tl.buffer)
	}
}

func TestDrainNewLines_CP949(t *testing.T) {
	raw, _ := korean.EUCKR.NewEncoder().String("심각: 연결 풀에서 커넥션을 얻지 못했습니다\n")
	tl := &Tailer{}
	tl.drainNewLines(bufio.NewReader(strings.NewReader(raw)), config.TailConfig{}, "")
	if len(tl.buffer) != 1 || tl.buffer[0].Line != "심각: 연결 풀에서 커넥션을 얻지 못했습니다" || tl.buffer[0].Level != "ERROR" {
		t.Fatalf("CP949 복원 실패: %+v", tl.buffer)
	}
}

func TestDrainNewLines_ReportableLevelPassesInclude(t *testing.T) {
	cfg := config.TailConfig{IncludePatterns: []string{"error", "fatal", "panic", "warning"}}
	in := "10:00 [main] WARN  c.e.App - pool exhausted\nSEVERE: boom\n[2026.11.12 10:15:22][0][b056] [container1-21] [SERVER-0242] Unable to get a connection\nINFO started\n"
	tl := &Tailer{}
	tl.drainNewLines(bufio.NewReader(strings.NewReader(in)), cfg, "")
	if len(tl.buffer) != 3 {
		t.Fatalf("WARN 이상 3줄만 기대: %+v", tl.buffer)
	}
}
