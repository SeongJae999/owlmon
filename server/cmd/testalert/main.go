// testalert는 이메일 알림 설정을 테스트하는 유틸리티입니다.
// 사용법: go run ./cmd/testalert
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/seongJae/owlmon/server/alert"
)

func main() {
	cfg := &alert.EmailConfig{
		Host:     getEnv("SMTP_HOST", ""),
		Port:     getEnv("SMTP_PORT", "587"),
		Username: getEnv("SMTP_USERNAME", ""),
		Password: getEnv("SMTP_PASSWORD", ""),
		From:     getEnv("SMTP_FROM", ""),
		To:       strings.Split(getEnv("SMTP_TO", ""), ","),
	}

	if cfg.Host == "" || cfg.Username == "" || cfg.Password == "" || cfg.From == "" || getEnv("SMTP_TO", "") == "" {
		fmt.Println("필수 환경변수를 설정하세요:")
		fmt.Println()
		fmt.Println("  Gmail 예시:")
		fmt.Println("    SMTP_HOST=smtp.gmail.com")
		fmt.Println("    SMTP_PORT=587")
		fmt.Println("    SMTP_USERNAME=your@gmail.com")
		fmt.Println("    SMTP_PASSWORD=앱비밀번호(16자리)")
		fmt.Println("    SMTP_FROM=your@gmail.com")
		fmt.Println("    SMTP_TO=수신자@email.com")
		fmt.Println()
		fmt.Println("  네이버 예시:")
		fmt.Println("    SMTP_HOST=smtp.naver.com")
		fmt.Println("    SMTP_PORT=587")
		fmt.Println("    SMTP_USERNAME=아이디")
		fmt.Println("    SMTP_PASSWORD=비밀번호")
		fmt.Println("    SMTP_FROM=아이디@naver.com")
		fmt.Println("    SMTP_TO=수신자@email.com")
		os.Exit(1)
	}

	log.Printf("테스트 이메일 발송 중... (수신자: %s)", strings.Join(cfg.To, ", "))
	err := cfg.Send(
		"🔔 OWLmon 알림 테스트",
		"이메일 알림 설정이 정상적으로 동작합니다.\n\n실제 알림 예시:\n- CPU 사용률이 90%를 초과하면 이 형식으로 발송됩니다.",
	)
	if err != nil {
		log.Fatalf("발송 실패: %v", err)
	}
	log.Println("발송 성공! 받은 편지함을 확인하세요.")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
