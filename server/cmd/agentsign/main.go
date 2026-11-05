package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	home, _ := os.UserHomeDir()
	keyPath := flag.String("key", filepath.Join(home, ".config/owlmon/agent-sign.key"), "개인키(seed hex) 경로")
	gen := flag.Bool("gen", false, "새 키 생성")
	flag.Parse()

	if *gen {
		if _, err := os.Stat(*keyPath); err == nil {
			log.Fatalf("이미 키가 있음: %s (덮어쓰면 기존 에이전트가 업데이트를 거부함)", *keyPath)
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(*keyPath), 0700); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*keyPath, []byte(hex.EncodeToString(priv.Seed())), 0600); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("개인키: %s\n공개키: %s\n", *keyPath, hex.EncodeToString(pub))
		return
	}

	if flag.NArg() != 1 {
		log.Fatal("사용법: agentsign [-key 경로] <binary>")
	}
	seedHex, err := os.ReadFile(*keyPath)
	if err != nil {
		log.Fatalf("개인키 읽기 실패: %v", err)
	}
	seed, err := hex.DecodeString(string(seedHex))
	if err != nil || len(seed) != ed25519.SeedSize {
		log.Fatalf("개인키 형식 오류: %s", *keyPath)
	}

	bin := flag.Arg(0)
	f, err := os.Open(bin)
	if err != nil {
		log.Fatal(err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}
	f.Close()

	// 서명 대상 = sha256 hex 문자열 (에이전트가 /api/agent/latest의 sha256으로 검증)
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), []byte(hex.EncodeToString(h.Sum(nil))))
	if err := os.WriteFile(bin+".sig", []byte(hex.EncodeToString(sig)), 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("서명 완료: %s.sig\n", bin)
}
