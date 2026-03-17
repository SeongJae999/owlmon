package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// 납품 시 관리자 비밀번호 해시 생성 유틸
// 사용법: go run ./cmd/hashpw <비밀번호>
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "사용법: hashpw <비밀번호>")
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "해시 생성 실패: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(hash))
	fmt.Fprintln(os.Stderr, "\n위 값을 OWLMON_PASSWORD_HASH 환경변수에 설정하세요.")
}
