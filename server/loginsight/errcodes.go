package loginsight

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type errCode struct {
	meaning string
	caution string
	handoff string
	source  string // 사람이 대조할 근거 — AI에는 보내지 않는다
}

// 신호·무해 구분은 일부러 두지 않는다 — AI에 "무해"가 가면 사전이 심각도를 정하게 된다
var errCodes = map[string]errCode{
	"TBR-2131":  {"프로그램과 Tibero 서버 사이 통신 중 입출력 오류. 접속 단계면 서버 미기동이나 접속 주소·포트 불일치", "디스크 장애로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-2000.cli.error.md https://docs.tibero.com/tibero-manuals/7.2.6.manuals/administrator-guide/appendix/appendix-d-troubleshooting.md"},
	"TBR-21004": {"테이블스페이스에서 공간(extent)을 더 할당하지 못함. 데이터파일이 가득 찼거나 단편화된 경우", "OS 디스크 용량 부족으로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-21000.tx.error.md https://support.tibero.com/hc/ko/articles/14376180164367"},
	"TBR-1063":  {"리두 로그가 모두 아카이브 완료를 기다려 로그 전환을 못 해 작업이 멈춤. 주로 아카이브 저장 공간 부족, 아카이브 지연으로도 생김", "리두 로그 파일 손상으로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-1000.cache.error.md https://support.tibero.com/hc/ko/articles/13836573407887"},
	"TBR-3018":  {"특정 용도의 공유 풀 메모리를 할당하지 못함", "함께 찍힌 Internal Error 문구로 버그 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-3000.common.error.md https://support.tibero.com/hc/ko/articles/13801670814735"},
	"TBR-3002":  {"공유 풀(SHP) 메모리 할당 실패. 공유 풀 크기 부족이나 단편화가 원인", "OS 메모리 부족으로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-3000.common.error.md https://support.tibero.com/hc/ko/articles/13896573252751"},
	"TBR-12001": {"서버가 세션을 닫음. 관리자 강제 종료나 시간 제한 외에 서버 쪽 내부 오류로 끊길 때도 찍힘", "단독이면 DB 다운으로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-12000.frame.error.md https://support.tibero.com/hc/ko/articles/13734613862415"},
	"TBR-12003": {"최대 세션 수에 도달해 새 접속을 받지 못함", "계정·비밀번호 문제 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-12000.frame.error.md"},
	"TBR-12033": {"NOWAIT 요청이나 DDL이 다른 세션·내부 작업이 잡은 잠금 때문에 기다리지 않고 바로 실패함", "파일 잠금·스토리지 문제 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-12000.frame.error.md https://support.tibero.com/hc/ko/articles/13801790345871"},
	"TBR-12098": {"같은 실행 계획·사전 객체를 쓰는 열린 커서 수가 한도를 넘음. 커서를 오래 열어둔 경우나 같은 SQL의 실행 계획이 반복 생성될 때 생김", "프로그램 결함만으로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-12000.frame.error.md https://support.tibero.com/hc/ko/articles/13866256160783"},
	"TBR-12099": {"서버가 제한 시간 안에 클라이언트 메시지를 받지 못해 연결을 닫음. 접속 인증 단계와 tbExport·tbImport 작업에서 보고됨", "서버 쪽 장애로 단정 아님", "네트워크 담당", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-12000.frame.error.md https://support.tibero.com/hc/ko/articles/14642827150607"},
	"TBR-12050": {"접속한 서버가 대기(standby) 상태이거나 아직 열리지 않아 다른 서버로 접속하라고 돌려보냄", "네트워크 장애로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-12000.frame.error.md https://support.tibero.com/hc/ko/articles/14654341545743"},
	"TBR-5074":  {"문자열을 숫자로 바꾸지 못함. 프로그램의 SQL이나 입력 값 문제", "DB 장애 아님", "프로그램 개발사", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-5000.datatype.error.md https://support.tibero.com/hc/ko/articles/13859029988879"},
	"TBR-8030":  {"INSERT 문의 값 개수가 대상 컬럼 수보다 적음. 프로그램 SQL 문제", "스키마 손상 아님", "프로그램 개발사", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-8000.dml.error.md"},
	"TBR-17001": {"DB 이름·사용자·비밀번호가 맞지 않아 로그인 실패", "단발은 입력 실수일 수 있어 공격으로 단정 아님", "프로그램 개발사", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-17000.secu.error.md"},
	"TBR-18017": {"원인이 분류되지 않은 입출력 오류. 사용자 인증 중 찍히면 상태 점검(health check)용 포트 접속일 수 있음", "이 번호만으로 디스크 장애로 단정 아님", "DB 유지보수 업체", "https://docs.tibero.com/tibero-manuals/7.2.6.manuals/error-reference-guide/chapter-18000.syscall.error.md https://support.tibero.com/hc/ko/articles/13753399920271"},

	"MSSQL-18456": {"로그인 실패. 머리 줄 State: 2·5 없는 로그인, 8 비밀번호 틀림, 38·46 요청한 DB 못 찾음, 58 인증 방식·SID 불일치", "모두 공격으로 단정 아님", "프로그램 개발사", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-18456-database-engine-error"},
	"MSSQL-824":   {"OS상 읽기·쓰기는 성공했으나 페이지 일관성 검사(체크섬 등)에 실패함. 대개 저장 장치·드라이버 경로 문제", "일시적 오류로 단정 아님", "서버·스토리지 담당", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-824-database-engine-error"},
	"MSSQL-823":   {"운영체제 수준의 읽기·쓰기 요청이 실패함. 메시지의 OS 오류 번호가 원인을 가리킴", "SQL Server 결함으로 단정 아님", "서버·스토리지 담당", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-823-database-engine-error"},
	"MSSQL-9002":  {"해당 DB의 트랜잭션 로그가 가득 차 데이터 변경이 막힘(읽기는 됨). 이유는 메시지의 due to 값(log_reuse_wait_desc)", "SQL Server 결함·SQL 문법 오류 아님", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-9002-database-engine-error https://learn.microsoft.com/en-us/sql/relational-databases/logs/troubleshoot-a-full-transaction-log-sql-server-error-9002"},
	"MSSQL-701":   {"SQL Server가 쿼리 실행에 쓸 메모리를 할당하지 못함. 다른 프로그램의 사용, SQL Server 내부 사용 등 원인이 여럿", "실패한 쿼리를 원인으로 단정 아님", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-701-database-engine-error"},
	"MSSQL-18452": {"검증할 수 없는 자격 증명이라 로그인 거부. 신뢰되지 않는 도메인 계정, Windows 인증 전용 서버에 SQL 로그인 등", "도메인 문제로만 단정 아님", "서버·도메인 담당", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-18452-database-engine-error"},
	"MSSQL-1204":  {"잠금 구조를 더 확보하지 못해 새 잠금을 잡지 못함. 메모리 부족이나 잠금 관련 설정 등이 원인", "교착 상태(1205)와 다른 오류", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-1204-database-engine-error"},
	"MSSQL-912":   {"업그레이드·누적 업데이트 뒤 첫 기동 때 DB 업그레이드 스크립트가 실패함. master에서 나면 인스턴스가 기동되지 않음", "원인은 912가 아닌 메시지 속 오류 번호", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-912-database-engine-error"},
	"MSSQL-3417":  {"master DB를 시작하지 못해 SQL Server가 실행되지 않음. 원인은 앞선 오류 줄", "이 줄만으로 원인 단정 아님", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-3417-database-engine-error"},
	"MSSQL-17182": {"기동 중 접속 수신부(프로토콜·포트·TLS 인증서 등) 초기화에 실패함. 원인은 Reason 뒤 문구", "케이블·방화벽 장애로 단정 아님", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/mssqlserver-17182-database-engine-error"},
	"MSSQL-18056": {"연결 풀 세션을 재사용하려고 초기화하다 실패함. State 29는 따로 분류되지 않은 예외를 뜻하는 일반 상태", "State 29만으로 원인 단정 아님", "DB 유지보수 업체", "https://learn.microsoft.com/en-us/archive/blogs/psssql/breaking-down-18056 https://learn.microsoft.com/en-us/archive/blogs/psssql/error-18056-can-be-unwanted-noise-in-certain-scenarios"},
	"MSSQL-17836": {"SQL 포트로 온 패킷의 길이 값이 실제 받은 양과 달라 연결을 끊음. 보안 점검·포트 스캔 사례가 많음. 먼저 볼 곳은 CLIENT 주소", "DB 서버 고장으로 단정 아님", "보안 담당", "https://blog.sqlauthority.com/2018/08/08/sql-server-fix-error-17836-length-specified-in-network-packet-payload-did-not-match-number-of-bytes-read-the-connection-has-been-closed/ https://kb.webtrends.com/problem/error-length-specified-in-network-packet-payload-did-not-match-number-of-bytes-read-1365448059643 https://www.sqlservercentral.com/forums/topic/advice-needed-troubleshooting-severity-20-errors"},
	"MSSQL-17573": {"DB가 시작될 때 그 DB에서 마지막으로 오류 없이 끝난 CHECKDB 시각을 알리는 정보 메시지", "지금 점검이 도는 것 아님", "", "https://learn.microsoft.com/en-us/archive/blogs/ialonso/scared-because-sql-server-seems-to-be-running-a-dbcc-checkdb-on-some-of-your-databases-on-every-startup https://www.sqlservercentral.com/blogs/does-sql-server-do-a-checkdb-during-an-instance-restart"},
	"MSSQL-17111": {"SQL Server가 메시지를 기록할 오류 로그 파일 경로를 알리는 정보 메시지", "오류 발생 알림 아님(경로 속 ERRORLOG 글자일 뿐)", "", "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors-16000-to-17999 https://learn.microsoft.com/en-us/answers/a/1282459"},
}

// 범용 [A-Z]+-\d+ 는 쓰지 않는다 — 티켓·작업 번호까지 잡힌다
var codeRules = []struct {
	re     *regexp.Regexp
	prefix string // 번호 묶음이 없는 문구 규칙이면 키 전체
}{
	// JDBC 드라이버도 서버 오류를 같은 번호로 찍는다 (드라이버 자체 오류는 90000번대라 사전에 없음)
	{regexp.MustCompile(`\b(?:TBR|JDBC)-(\d{1,6})\b`), "TBR-"},
	// sys.log는 ec=ERROR_상수(-번호) — 인증 실패 줄은 ec= 없이 찍힌다
	{regexp.MustCompile(`\bERROR_[A-Z0-9_]+\(-(\d{1,6})\)`), "TBR-"},
	// ", Severity:"까지 봐야 일반 앱 로그의 "Error: 404"를 잡지 않는다
	{regexp.MustCompile(`\bError: (\d{1,6}), Severity:`), "MSSQL-"},
	{regexp.MustCompile(`CHECKDB for database (?:'[^']*'|<STR>) finished without errors`), "MSSQL-17573"},
	{regexp.MustCompile(`Logging SQL Server messages in file`), "MSSQL-17111"},
}

type codeHit struct {
	key        string
	pos        int
	start, end int // 번호 위치, 문구 규칙이면 -1
}

func findCodes(line string) []codeHit {
	var hits []codeHit
	for _, r := range codeRules {
		for _, m := range r.re.FindAllStringSubmatchIndex(line, -1) {
			h := codeHit{key: r.prefix, pos: m[0], start: -1, end: -1}
			if len(m) > 2 {
				h.key += line[m[2]:m[3]]
				h.start, h.end = m[2], m[3]
			}
			if _, ok := errCodes[h.key]; ok && !inUntrustedToken(line, m[0]) {
				hits = append(hits, h)
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	return hits
}

// 경로·계정명 토큰 속 코드는 외부에서 넣을 수 있다 — 한 마디 URL(GET /x-TBR-1063), 윈도 경로,
// 경로·계정명 바로 뒤에 붙인 코드(<PATH>:TBR-1063, <USER>,TBR-1063)
// ponytail: 슬래시 없는 입력값(Spring "No static resource TBR-1063.")은 못 거른다. 지시문(ErrCodeRules)에 기댄다
func inUntrustedToken(line string, pos int) bool {
	start := 0
	if i := strings.LastIndexFunc(line[:pos], unicode.IsSpace); i >= 0 {
		_, n := utf8.DecodeRuneInString(line[i:])
		start = i + n
	}
	end := len(line)
	if i := strings.IndexFunc(line[pos:], unicode.IsSpace); i >= 0 {
		end = pos + i
	}
	tok := line[start:end]
	return strings.ContainsAny(tok, `/\`) || strings.Contains(tok, "<PATH>") || strings.Contains(tok, "<USER>")
}

func errCodeDictOn() bool {
	return os.Getenv("OWLMON_LOGINSIGHT_ERRCODE_DICT") != "off"
}

func errCodeOff(key string) bool {
	for _, k := range strings.Split(os.Getenv("OWLMON_LOGINSIGHT_ERRCODE_OFF"), ",") {
		if strings.TrimSpace(k) == key {
			return true
		}
	}
	return false
}

func errCodeLine(key string) string {
	c := errCodes[key]
	s := key + ": " + c.meaning + ". " + c.caution
	if c.handoff != "" {
		s += ". 넘길 곳: " + c.handoff
	}
	return s
}

// 샘플 3줄에서 5개를 넘는 일은 드물고, 요청 경로 등에 코드를 잔뜩 넣어 해설을 끌어오는 것도 막는다
const maxErrCodeNotes = 5

func errCodeNotes(lines []string) string {
	if !errCodeDictOn() {
		return ""
	}
	seen := map[string]bool{}
	var b strings.Builder
	n := 0
	for _, l := range lines {
		// 원문이 아니라 템플릿에서 찾는다 — 따옴표·계정명·경로 속 코드는 공격자가 넣을 수 있는 자리다
		for _, h := range findCodes(Templatize(l)) {
			if n == maxErrCodeNotes || seen[h.key] || errCodeOff(h.key) {
				continue
			}
			seen[h.key] = true
			b.WriteString("- " + errCodeLine(h.key) + "\n")
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return "[오류 코드 해설] (제조사 문서 요약 — 코드의 일반적 뜻이며 이번 원인 확정 아님)\n" + b.String()
}

// 캐시 판정용 사전 상태 — TemplateHash에 섞으면 해설을 고치거나 끌 때마다 [패턴 이력]과 화면 카드가 갈라진다.
// 샘플 줄로 만들면 원인 묶음(IncidentKey)처럼 샘플 구성이 tick마다 바뀌는 묶음의 24h 캐시가 깨진다
func errCodeStateSig() string {
	if !errCodeDictOn() {
		return "off"
	}
	keys := make([]string, 0, len(errCodes))
	for k := range errCodes {
		if errCodeOff(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys) // OFF 값의 공백·순서·오타만 바뀌었을 때 캐시 전체를 버리지 않게
	return ErrCodeDigest() + "|" + strings.Join(keys, ",")
}

// 해설을 꺼 둔 코드도 포함 — 끈 동안 만든 분석이 다시 켠 뒤 24시간 재사용되지 않게 서명을 남긴다
func hasErrCodes(lines []string) bool {
	for _, l := range lines {
		if len(findCodes(Templatize(l))) > 0 {
			return true
		}
	}
	return false
}

// 해설 없이 만든 분석은 낡을 해설이 없어 재사용해도 되고, 해설을 붙인 분석은 사전 상태가 같을 때만 재사용한다
func errCodeCacheOK(cachedSig string) bool {
	return cachedSig == "" || cachedSig == errCodeStateSig()
}

// 사전에 있는 번호는 <N>으로 지우지 않는다 — 지우면 9002와 18456 같은 다른 오류가 한 캐시로 합쳐진다.
// 사전 밖 번호까지 남기면 묶음이 끝없이 늘어나 사전에 있는 것만 남긴다.
// 뒤따르는 치환 정규식이 건드리지 못하게 숫자를 사용자 영역 문자 + g~p 글자로 잠시 바꿔 둔다
const codeMark = "\ue000"

var reCodeMark = regexp.MustCompile(codeMark + `([g-p]+)` + codeMark)

func protectCodes(s string) string {
	if !errCodeDictOn() {
		return s
	}
	hits := findCodes(s)
	for i := len(hits) - 1; i >= 0; i-- {
		h := hits[i]
		if h.start < 0 {
			continue
		}
		enc := strings.Map(func(r rune) rune { return r - '0' + 'g' }, s[h.start:h.end])
		s = s[:h.start] + codeMark + enc + codeMark + s[h.end:]
	}
	return s
}

func restoreCodes(s string) string {
	return reCodeMark.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Map(func(r rune) rune { return r - 'g' + '0' }, strings.Trim(m, codeMark))
	})
}

// ErrCodeDigest — 평가 결과에 어느 사전으로 쟀는지 남기려는 해시
func ErrCodeDigest() string {
	keys := make([]string, 0, len(errCodes))
	for k := range errCodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(errCodeLine(k) + "\n"))
	}
	for _, r := range codeRules {
		h.Write([]byte(r.re.String() + r.prefix + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil)[:4])
}
