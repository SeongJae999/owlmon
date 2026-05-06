package dpm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Cipher는 AES-256-GCM 기반 비밀번호 암복호화 도구입니다.
// 마스터 키는 환경변수(OWLMON_DPM_KEY)에서 받아 SHA-256으로 32바이트 키를 파생합니다.
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher는 마스터 키 문자열로부터 Cipher를 생성합니다.
func NewCipher(masterKey string) (*Cipher, error) {
	if masterKey == "" {
		return nil, errors.New("OWLMON_DPM_KEY 환경변수가 비어있습니다")
	}
	hash := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

// Encrypt는 평문을 암호화하고 base64로 인코딩하여 반환합니다.
func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := c.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt는 base64 암호문을 복호화하여 평문을 반환합니다.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(data) < c.gcm.NonceSize() {
		return "", errors.New("암호문이 너무 짧음")
	}
	nonce, ct := data[:c.gcm.NonceSize()], data[c.gcm.NonceSize():]
	plain, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
