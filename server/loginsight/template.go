package loginsight

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Drain-lite 정규식 — 자주 변하는 토큰을 placeholder로 치환.
// 순서 중요: 더 구체적인 패턴(타임스탬프, UUID, IP)을 먼저, 그 다음 PATH, 마지막에 NUM.
var (
	reTimestamp = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?\b`)
	reUUID      = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	reIP        = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?\b`)
	rePath      = regexp.MustCompile(`(?:/[A-Za-z0-9._-]+){2,}`) // 최소 2-depth만 PATH로 간주
	reHex       = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`)
	reBareHex   = regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`) // 0x 없는 ID (Proxmox UPID 0036E1B3, 컨테이너 ID 등)
	reQuoted    = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	reNum       = regexp.MustCompile(`\b\d+\b`)
)

// 인증 로그 계정명 마스킹 — auth 문맥(sshd/pam 관용구)에 한정해 과도한 일반화를 피한다.
// 계정명만 다른 brute-force 로그(root/admin/oracle...)가 같은 그룹으로 묶이도록 함.
var (
	// "Failed password for root", "Accepted publickey for bob",
	// "Failed password for invalid user oracle" (sshd)
	reAuthFor = regexp.MustCompile(`(?i)\b((?:Failed|Accepted|Postponed) \S+ for (?:invalid user )?)([^\s,;:]+)`)
	// "Invalid user admin from ..." (sshd)
	reInvalidUser = regexp.MustCompile(`(?i)\b(Invalid user )([^\s,;:]+)`)
	// "session opened for user root by ..." (pam)
	reForUser = regexp.MustCompile(`(?i)\b(for user )([^\s,;:]+)`)
	// "authentication failure; ... user=root" (pam)
	reUserEq = regexp.MustCompile(`(?i)\b(user=)([^\s,;:]+)`)
)

// 원인 묶기 — 템플릿이 달라도 같은 원인(같은 대상 IP 연결 실패, 같은 systemd 유닛)이면 한 그룹으로.
var (
	reConnFail = regexp.MustCompile(`(?i)timed? ?out|refused|no route to host|unreachable|can'?t connect|cannot connect|could not connect|unable to connect|failed to connect|not online|not responding`)
	// "to <IP>"만 대상으로 본다 — "from/by <IP>"는 공격 출발지 등이라 원인으로 묶으면 안 됨
	reTargetIP = regexp.MustCompile(`(?i)\bto\s+['"]?((?:\d{1,3}\.){3}\d{1,3})\b`)
	reUnit     = regexp.MustCompile(`\b[A-Za-z0-9@_.-]+\.(?:service|timer|socket|mount)\b`)
	reDigits   = regexp.MustCompile(`\d+`)
)

// IncidentKey — 원인 묶기 키 ("target:<IP>" 또는 "unit:<유닛>"). 해당 없으면 "".
func IncidentKey(line string) string {
	if reConnFail.MatchString(line) {
		if m := reTargetIP.FindStringSubmatch(line); m != nil {
			return "target:" + m[1]
		}
	}
	if u := reUnit.FindString(line); u != "" {
		return "unit:" + reDigits.ReplaceAllString(u, "N") // run-u123.service 같은 일회성 유닛도 한 키로
	}
	return ""
}

// Templatize — 로그 한 줄에서 가변 부분을 placeholder로 치환한 템플릿 문자열을 반환.
//
// 치환 순서:
//  1. <USER>  인증 로그 계정명 (sshd/pam 관용구 한정 — 숫자 포함 계정명이 <N>으로 깨지기 전에 먼저)
//  2. <TS>    타임스탬프 (ISO 8601 등)
//  3. <UUID>  UUID v4
//  4. <IP>    IPv4[:port]
//  5. <PATH>  /a/b 형태 2-depth 이상 경로
//  6. <HEX>   0xDEADBEEF, 0x 없는 8자 이상 16진 ID (숫자+a-f 혼합)
//  7. <STR>   "..." 또는 '...' 인용 문자열
//  8. <N>     일반 정수
//
// 같은 패턴의 로그가 같은 템플릿을 산출해야 클러스터링이 의미를 갖는다.
func Templatize(line string) string {
	s := strings.TrimSpace(line)
	s = reAuthFor.ReplaceAllString(s, "${1}<USER>")
	s = reInvalidUser.ReplaceAllString(s, "${1}<USER>")
	s = reForUser.ReplaceAllString(s, "${1}<USER>")
	s = reUserEq.ReplaceAllString(s, "${1}<USER>")
	s = reTimestamp.ReplaceAllString(s, "<TS>")
	s = reUUID.ReplaceAllString(s, "<UUID>")
	s = reIP.ReplaceAllString(s, "<IP>")
	s = rePath.ReplaceAllString(s, "<PATH>")
	s = reHex.ReplaceAllString(s, "<HEX>")
	s = reBareHex.ReplaceAllStringFunc(s, bareHex)
	s = reQuoted.ReplaceAllString(s, "<STR>")
	s = reNum.ReplaceAllString(s, "<N>")
	// 공백 정규화 (다중 공백 → 단일 공백)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// bareHex — 숫자와 a-f가 섞인 토큰만 <HEX>. 순수 숫자는 <N>으로, 영단어(deadbeef 등)는 그대로 둔다.
func bareHex(tok string) string {
	if strings.ContainsAny(tok, "0123456789") && strings.ContainsAny(tok, "abcdefABCDEF") {
		return "<HEX>"
	}
	return tok
}

// Hash — 템플릿 문자열의 16자(64bit) 해시.
// 같은 템플릿은 항상 같은 해시 → 캐시 키로 사용 가능.
func Hash(template string) string {
	h := sha256.Sum256([]byte(template))
	return hex.EncodeToString(h[:8])
}

// LogLine — Cluster 입력용. 워커가 logs 테이블에서 fetch한 raw 로그를 이 형태로 변환한다.
type LogLine struct {
	ID       int64
	HostName string
	Message  string
}

// Cluster — 입력 로그를 (호스트, 원인 키 또는 템플릿 해시) 기준으로 그룹화한다.
//
// 같은 호스트 + 같은 템플릿 = 하나의 LogGroup.
// 호스트가 다르면 같은 패턴이라도 별도 그룹 (운영자가 호스트별 대응 필요).
//
// 각 그룹은 샘플 라인을 최대 maxSamples건만 보관 (LLM 토큰 절약).
// 결과는 입력 순서대로 (테스트 안정성).
func Cluster(lines []LogLine, maxSamples int) []LogGroup {
	if maxSamples <= 0 {
		maxSamples = 3
	}
	type key struct {
		host, hash string
	}
	type sampling struct {
		seen map[string]bool // 이미 샘플에 있는 템플릿
		dups []int           // 같은 템플릿이 반복된 샘플 위치 (새 템플릿이 오면 교체 대상)
	}
	groups := map[key]*LogGroup{}
	samples := map[key]*sampling{}
	order := []key{}

	for _, l := range lines {
		tpl := Templatize(l.Message)
		th := Hash(tpl)
		ik := IncidentKey(l.Message)
		h := th
		if ik != "" {
			h = Hash(ik)
		}
		k := key{host: l.HostName, hash: h}
		g, ok := groups[k]
		if !ok {
			g = &LogGroup{
				TemplateHash:    h,
				TemplateExample: tpl,
				HostName:        l.HostName,
				IncidentKey:     ik,
			}
			groups[k] = g
			samples[k] = &sampling{seen: map[string]bool{}}
			order = append(order, k)
		}
		g.Count++
		sp := samples[k]
		switch {
		case len(g.SampleLines) < maxSamples:
			if sp.seen[th] {
				sp.dups = append(sp.dups, len(g.SampleLines))
			}
			sp.seen[th] = true
			g.SampleLines = append(g.SampleLines, l.Message)
			g.SampleLogIDs = append(g.SampleLogIDs, l.ID)
		case !sp.seen[th] && len(sp.dups) > 0:
			// 묶인 그룹의 다른 종류 로그도 LLM이 보도록 반복 샘플을 교체
			i := sp.dups[len(sp.dups)-1]
			sp.dups = sp.dups[:len(sp.dups)-1]
			sp.seen[th] = true
			g.SampleLines[i], g.SampleLogIDs[i] = l.Message, l.ID
		}
	}

	result := make([]LogGroup, 0, len(order))
	for _, k := range order {
		result = append(result, *groups[k])
	}
	return result
}
