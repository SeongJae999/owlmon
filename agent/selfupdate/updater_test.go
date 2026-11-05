package selfupdate

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	sha := "abc123"
	sig := hex.EncodeToString(ed25519.Sign(priv, []byte(sha)))

	if err := verifySignature(pubHex, sha, sig); err != nil {
		t.Fatalf("정상 서명 거부: %v", err)
	}
	if verifySignature(pubHex, "tampered", sig) == nil {
		t.Fatal("다른 sha256에 같은 서명 → 거부돼야 함")
	}
	if verifySignature(pubHex, sha, "") == nil {
		t.Fatal("서명 없음 → 거부돼야 함")
	}
}

// 내장 공개키 오타 방지 — 32바이트 ed25519 키여야 함.
func TestSigningPublicKeyValid(t *testing.T) {
	b, err := hex.DecodeString(signingPublicKey)
	if err != nil || len(b) != ed25519.PublicKeySize {
		t.Fatalf("공개키 형식 오류: len=%d err=%v", len(b), err)
	}
}
