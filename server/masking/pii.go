// Package masking은 로그 라인에서 개인정보(PII)를 마스킹한다.
// 한국 개인정보보호법 + 학교/공공기관 보안 지침을 기준으로 한다.
//
// 처리 위치:
//   - 서버 ingest 직전 (DB에 저장될 라인이 마스킹됨)
//   - LLM 입력 직전 (모델에 평문 PII 전달 금지)
//
// 정책:
//   - 주민번호/카드/계좌/여권/면허 → 완전 마스킹 ([RRN-MASKED] 등)
//   - 이메일/전화번호                → 부분 마스킹 (식별 가능성 낮춤)
//   - IP: RFC1918 사내망은 유지, 외부 IP는 마스킹 (옵션 토글)
package masking

import (
	"net"
	"regexp"
	"strings"
)

// MaskOptions는 마스킹 동작을 제어한다.
type MaskOptions struct {
	MaskRRN          bool // 주민등록번호
	MaskPassport     bool // 여권번호
	MaskDriverLic    bool // 운전면허
	MaskCard         bool // 카드번호
	MaskAccount      bool // 계좌번호
	MaskEmail        bool // 이메일
	MaskPhone        bool // 전화번호
	MaskExternalIP   bool // 외부(공인) IP — RFC1918 사내망은 유지
}

// DefaultOptions는 운영 환경 권장 기본값이다.
func DefaultOptions() MaskOptions {
	return MaskOptions{
		MaskRRN:        true,
		MaskPassport:   true,
		MaskDriverLic:  true,
		MaskCard:       true,
		MaskAccount:    true,
		MaskEmail:      true,
		MaskPhone:      true,
		MaskExternalIP: false, // 기본 OFF — 운영 디버깅에 IP 필요. 명시적 ON.
	}
}

// 컴파일된 정규식 (init에서 한 번만 컴파일, 이후 재사용)
var (
	reRRN       = regexp.MustCompile(`\b\d{6}-[1-4]\d{6}\b`)
	rePassport  = regexp.MustCompile(`\b[A-Z][0-9]{8}\b`)
	reDriverLic = regexp.MustCompile(`\b\d{2}-\d{2}-\d{6}-\d{2}\b`)
	reCard      = regexp.MustCompile(`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`)
	reAccount   = regexp.MustCompile(`\b\d{3,6}-\d{2,6}-\d{2,7}\b`)
	reEmail     = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	rePhone     = regexp.MustCompile(`\b01[016789]-?\d{3,4}-?\d{4}\b`)
	reIPv4      = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
)

// Mask는 입력 라인의 PII를 옵션에 따라 마스킹해서 반환한다.
// 정규식 false positive 줄이려 카드/계좌 등 자릿수 엄격하게 정의.
func Mask(line string, opts MaskOptions) string {
	if opts.MaskRRN {
		line = reRRN.ReplaceAllString(line, "[RRN-MASKED]")
	}
	if opts.MaskPassport {
		line = rePassport.ReplaceAllString(line, "[PASSPORT-MASKED]")
	}
	if opts.MaskDriverLic {
		line = reDriverLic.ReplaceAllString(line, "[LICENSE-MASKED]")
	}
	if opts.MaskCard {
		line = reCard.ReplaceAllString(line, "[CARD-MASKED]")
	}
	// 전화번호를 계좌번호보다 먼저 — 전화번호 형식이 계좌 정규식에 false positive로 잡히는 것 방지
	if opts.MaskPhone {
		line = rePhone.ReplaceAllString(line, "010-****-****")
	}
	if opts.MaskAccount {
		line = reAccount.ReplaceAllString(line, "[ACCOUNT-MASKED]")
	}
	if opts.MaskEmail {
		line = reEmail.ReplaceAllStringFunc(line, maskEmail)
	}
	if opts.MaskExternalIP {
		line = reIPv4.ReplaceAllStringFunc(line, maskIPIfExternal)
	}
	return line
}

// maskEmail은 user@domain.com → u***@domain.com 형태로 부분 마스킹.
// 도메인은 유지 (운영 디버깅 시 어떤 회사 메일인지 식별 필요).
func maskEmail(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at <= 1 {
		return "[EMAIL-MASKED]"
	}
	local := addr[:at]
	domain := addr[at:]
	if len(local) <= 1 {
		return "*" + domain
	}
	return string(local[0]) + strings.Repeat("*", len(local)-1) + domain
}

// maskIPIfExternal은 IPv4에서 RFC1918 사내망(10/8, 172.16/12, 192.168/16) +
// loopback(127/8) + link-local(169.254/16)은 유지, 외부 IP만 마스킹.
// 외부 IP는 마지막 2옥텟을 가려서 식별 가능성을 낮춤 (예: 1.2.3.4 → 1.2.x.x).
func maskIPIfExternal(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return s
	}
	if isPrivate(ip4) {
		return s
	}
	// 첫 2옥텟 유지, 나머지 가림 — ISP/지역 식별 가능성은 유지하면서 호스트는 가림
	return strings.Join([]string{
		itoa(int(ip4[0])),
		itoa(int(ip4[1])),
		"x", "x",
	}, ".")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// isPrivate는 RFC1918 + loopback + link-local 여부.
func isPrivate(ip net.IP) bool {
	switch {
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	case ip[0] == 127:
		return true
	case ip[0] == 169 && ip[1] == 254:
		return true
	}
	return false
}
