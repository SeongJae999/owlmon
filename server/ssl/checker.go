package ssl

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

// CertInfo는 SSL 인증서 상태 정보입니다.
type CertInfo struct {
	Domain    string    `json:"domain"`
	Port      int       `json:"port"`
	Issuer    string    `json:"issuer"`
	NotAfter  time.Time `json:"not_after"`
	DaysLeft  int       `json:"days_left"`
	Status    string    `json:"status"` // ok, warning, critical, expired, error
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// CheckCert는 TLS 핸드쉐이크로 인증서 만료일을 확인합니다.
func CheckCert(domain string, port int) CertInfo {
	info := CertInfo{
		Domain:    domain,
		Port:      port,
		CheckedAt: time.Now(),
	}

	addr := fmt.Sprintf("%s:%d", domain, port)
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp", addr,
		&tls.Config{
			// 만료된 인증서도 정보를 가져오기 위해 검증 건너뜀
			InsecureSkipVerify: true,
		},
	)
	if err != nil {
		info.Status = "error"
		info.Error = err.Error()
		return info
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		info.Status = "error"
		info.Error = "인증서를 찾을 수 없습니다"
		return info
	}

	cert := certs[0]
	info.Issuer = cert.Issuer.CommonName
	if info.Issuer == "" {
		info.Issuer = cert.Issuer.Organization[0]
	}
	info.NotAfter = cert.NotAfter
	info.DaysLeft = int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))

	// 학교/공공기관 결재·구매 lead time 고려한 단계화
	//   ≤ 0일:  expired   (만료됨)
	//   ≤ 7일:  critical  (긴급 갱신)
	//   ≤ 30일: warning   (갱신 임박)
	//   ≤ 60일: notice    (갱신 준비 — 결재 시작 시점)
	//   > 60일: ok        (정상)
	switch {
	case info.DaysLeft <= 0:
		info.Status = "expired"
	case info.DaysLeft <= 7:
		info.Status = "critical"
	case info.DaysLeft <= 30:
		info.Status = "warning"
	case info.DaysLeft <= 60:
		info.Status = "notice"
	default:
		info.Status = "ok"
	}

	return info
}

// CertChecker는 등록된 도메인의 인증서 상태를 주기적으로 체크합니다.
type CertChecker struct {
	mu      sync.RWMutex
	results []CertInfo
}

func NewCertChecker() *CertChecker {
	return &CertChecker{}
}

// CheckAll은 도메인 목록의 인증서를 모두 체크하고 결과를 캐시합니다.
func (cc *CertChecker) CheckAll(domains []DomainEntry) []CertInfo {
	results := make([]CertInfo, len(domains))
	var wg sync.WaitGroup
	for i, d := range domains {
		wg.Add(1)
		go func(idx int, domain DomainEntry) {
			defer wg.Done()
			results[idx] = CheckCert(domain.Domain, domain.Port)
		}(i, d)
	}
	wg.Wait()

	cc.mu.Lock()
	cc.results = results
	cc.mu.Unlock()

	return results
}

// GetResults는 캐시된 체크 결과를 반환합니다.
func (cc *CertChecker) GetResults() []CertInfo {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	out := make([]CertInfo, len(cc.results))
	copy(out, cc.results)
	return out
}

// DomainEntry는 체크할 도메인 정보입니다.
type DomainEntry struct {
	ID     int64
	Domain string
	Port   int
}
