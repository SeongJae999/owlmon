package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/service"
)

func main() {
	if service.IsService() {
		// 서비스 모드: 로그를 파일로 저장
		logFile, err := os.OpenFile(`C:\owlmon-server\service.log`,
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
		log.Println("Windows 서비스 시작됨")
		if err := service.Run(startServer); err != nil {
			log.Fatalf("서비스 실행 실패: %v", err)
		}
		return
	}
	// 콘솔 모드: 시그널 대기
	stop := startServer()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("OWLmon 서버 종료 중...")
	stop()
}

// startServer는 서버를 시작하고 정지 함수를 반환합니다.
func startServer() func() {
	_ = godotenv.Overload()

	username := getEnv("OWLMON_USERNAME", "admin")
	passwordHash := getEnv("OWLMON_PASSWORD_HASH", "")
	prometheusURL := getEnv("OWLMON_PROMETHEUS_URL", "http://localhost:9090")
	listenAddr := getEnv("OWLMON_LISTEN", ":8080")

	if passwordHash == "" {
		log.Fatal("OWLMON_PASSWORD_HASH 환경변수가 설정되지 않았습니다.")
	}

	// 1. JWT Secret Persistent Logic
	jwtSecret := getEnv("OWLMON_JWT_SECRET", "")
	secretFile := ".owlmon_secret"
	if jwtSecret == "" || jwtSecret == "change-this-secret-in-production" {
		// Try reading from file
		if b, err := os.ReadFile(secretFile); err == nil {
			jwtSecret = string(b)
			log.Println("🔑 Persistent JWT secret loaded from file")
		} else {
			// Generate and save new secret
			b := make([]byte, 32)
			rand.Read(b)
			jwtSecret = hex.EncodeToString(b)
			os.WriteFile(secretFile, []byte(jwtSecret), 0600)
			log.Println("✨ New persistent JWT secret generated and saved")
		}
	}

	// 2. DB 및 저장소 초기화
	appCtx := InitDB()

	// 3. 알림 체커 및 이메일 설정
	smtpHost := getEnv("SMTP_HOST", "")
	var emailCfg *alert.EmailConfig
	if smtpHost != "" {
		emailCfg = &alert.EmailConfig{
			Host:     smtpHost,
			Port:     getEnv("SMTP_PORT", "587"),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     getEnv("SMTP_FROM", ""),
			To:       []string{getEnv("SMTP_TO", "")},
		}
	}
	checker := alert.NewChecker(prometheusURL, emailCfg, appCtx.ConfigStore, appCtx.HistorySaver)

	// 4. 백그라운드 워커 시작
	InitWorkers(appCtx, checker, emailCfg, prometheusURL)

	// 5. 라우터 설정
	r := InitRouter(appCtx, checker, jwtSecret, username, passwordHash, prometheusURL)

	tlsCert := getEnv("OWLMON_TLS_CERT", "")
	tlsKey := getEnv("OWLMON_TLS_KEY", "")

	srv := &http.Server{Addr: listenAddr, Handler: r}

	go func() {
		log.Printf("OWLmon 서버 시작: %s", listenAddr)
		var err error
		if tlsCert != "" && tlsKey != "" {
			cert, loadErr := tls.LoadX509KeyPair(tlsCert, tlsKey)
			if loadErr == nil {
				srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
				err = srv.ListenAndServeTLS("", "")
			} else {
				log.Fatalf("TLS 로드 실패: %v", loadErr)
			}
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("서버 시작 실패: %v", err)
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("서버 종료 실패: %v", err)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
