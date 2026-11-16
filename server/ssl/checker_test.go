package ssl

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// 도메인이 많아도 동시에 여는 접속은 상한을 넘지 않고, 결과는 입력 순서대로 나와야 함
func TestCheckAllLimitsConcurrency(t *testing.T) {
	var cur, peak atomic.Int32
	checkCert = func(domain string, port int) CertInfo {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		cur.Add(-1)
		return CertInfo{Domain: domain, Port: port}
	}
	defer func() { checkCert = CheckCert }()

	domains := make([]DomainEntry, 100)
	for i := range domains {
		domains[i] = DomainEntry{Domain: fmt.Sprintf("d%d.example", i), Port: 443}
	}
	got := NewCertChecker().CheckAll(domains)

	if p := peak.Load(); p > maxConcurrentChecks || p < 2 {
		t.Fatalf("동시 접속 최대 %d — 1 초과 %d 이하여야 함", p, maxConcurrentChecks)
	}
	for i, r := range got {
		if r.Domain != domains[i].Domain {
			t.Fatalf("%d번 결과가 %s — 입력 순서와 다름", i, r.Domain)
		}
	}
}
