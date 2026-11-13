package logtail

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// MSSQL ERRORLOG 등 UTF-16LE(BOM FF FE) 파일은 풀어 읽어야 키워드가 매칭됨. 끝으로 Seek한 뒤에도 BOM은 0번지에서 확인
func newLineReader(f *os.File) *bufio.Reader {
	bom := make([]byte, 2)
	if n, _ := f.ReadAt(bom, 0); n == 2 && bom[0] == 0xFF && bom[1] == 0xFE {
		return bufio.NewReader(transform.NewReader(f, xunicode.UTF16(xunicode.LittleEndian, xunicode.UseBOM).NewDecoder()))
	}
	return bufio.NewReader(f)
}

// ponytail: UTF-8이 아닌 줄은 CP949(한국어 Windows의 JEUS·Tomcat 기본)로 가정 — 그 밖의 레거시 인코딩은 깨짐
func decodeLine(line string) string {
	if utf8.ValidString(line) {
		return line
	}
	if s, err := korean.EUCKR.NewDecoder().String(line); err == nil {
		return s
	}
	return line
}

// 서버 loginsight 필터(error·warn·fatal)와 log_store 집계가 이 값을 그대로 쓴다
var levelWords = map[string]string{
	"FATAL": "FATAL", "PANIC": "FATAL", "CRIT": "FATAL", "CRITICAL": "FATAL",
	"EMERG": "FATAL", "EMERGENCY": "FATAL", "ALERT": "FATAL",
	"ERROR": "ERROR", "SEVERE": "ERROR", "심각": "ERROR", "오류": "ERROR", "에러": "ERROR",
	"WARN": "WARN", "WARNING": "WARN", "경고": "WARN",
	"INFO": "INFO", "DEBUG": "DEBUG",
}

var levelRank = map[string]int{"DEBUG": 1, "INFO": 2, "WARN": 3, "ERROR": 4, "FATAL": 5}

var (
	// JEUS: [날짜 시각][레벨 숫자] — 0=SEVERE, 1=WARNING, 2=INFO
	jeusLevel  = regexp.MustCompile(`^\[\d{4}\.\d{2}\.\d{2} [\d:]+\]\[(\d)\]`)
	tiberoCode = regexp.MustCompile(`\bTBR-\d+`)
)

func reportable(level string) bool { return levelRank[level] >= levelRank["WARN"] }

func detectLevel(line string) string {
	if m := jeusLevel.FindStringSubmatch(line); m != nil {
		return [...]string{"ERROR", "WARN", "INFO"}[min(int(m[1][0]-'0'), 2)]
	}
	implicit := ""
	if strings.Contains(line, "TBR-") && tiberoCode.MatchString(line) {
		implicit = "ERROR"
	}
	for _, field := range strings.Fields(line) {
		// 경로·URL·로그 파일명 안의 error는 레벨이 아님 (/var/log/nginx/error.log, GET /api/error)
		if strings.ContainsAny(field, `/\`) || strings.Contains(field, ".log") {
			continue
		}
		words := strings.FieldsFunc(field, func(r rune) bool { return !unicode.IsLetter(r) })
		for i, word := range words {
			upper := strings.ToUpper(word)
			lvl, ok := levelWords[upper]
			// 대문자(INFO)·대괄호([error], [core:warn]) 표기는 로거가 선언한 레벨이라 맨 앞 것을 따름
			if ok && (word == upper || i == len(words)-1 && strings.HasSuffix(field, "]")) {
				return lvl
			}
			if !ok && len(word) > len("Error") && strings.HasSuffix(word, "Error") { // OutOfMemoryError 등
				lvl, ok = "ERROR", true
			}
			// 본문 속 alert는 "alert mail" 같은 일반 단어라 제외
			if ok && upper != "ALERT" && levelRank[lvl] > levelRank[implicit] {
				implicit = lvl
			}
		}
	}
	return implicit
}
