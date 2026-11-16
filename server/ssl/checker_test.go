package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
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

// 발급자 CN·O가 모두 빈 인증서에서 panic 나면 서버 프로세스 전체가 죽음
func TestCheckCertEmptyIssuer(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()

	info := CheckCert("127.0.0.1", ln.Addr().(*net.TCPAddr).Port)
	if info.Status == "error" || info.Issuer != "" {
		t.Fatalf("got status=%q issuer=%q err=%q", info.Status, info.Issuer, info.Error)
	}
}

// checkCert가 panic해도 CheckAll은 그 도메인만 오류로 표시하고 계속 진행해야 함
func TestCheckAllRecoversPanic(t *testing.T) {
	checkCert = func(domain string, port int) CertInfo {
		if domain == "bad" {
			panic("boom")
		}
		return CertInfo{Domain: domain, Status: "ok"}
	}
	defer func() { checkCert = CheckCert }()

	res := NewCertChecker().CheckAll([]DomainEntry{{Domain: "bad", Port: 443}, {Domain: "good", Port: 443}})
	if res[0].Status != "error" || res[0].Domain != "bad" || res[1].Status != "ok" {
		t.Fatalf("got %+v", res)
	}
}
