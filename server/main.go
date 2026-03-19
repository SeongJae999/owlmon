package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/handler"
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
	// .env 파일 로드 (.env가 환경변수보다 우선, 없으면 무시)
	_ = godotenv.Overload()

	username := getEnv("OWLMON_USERNAME", "admin")
	passwordHash := getEnv("OWLMON_PASSWORD_HASH", "")
	jwtSecret := getEnv("OWLMON_JWT_SECRET", "change-this-secret-in-production")
	prometheusURL := getEnv("OWLMON_PROMETHEUS_URL", "http://localhost:9090")
	listenAddr := getEnv("OWLMON_LISTEN", ":8080")

	if passwordHash == "" {
		log.Fatal("OWLMON_PASSWORD_HASH 환경변수가 설정되지 않았습니다.\n" +
			"다음 명령어로 해시를 생성하세요:\n" +
			"  go run ./cmd/hashpw <비밀번호>")
	}

	// 알림 설정 저장소 (OWLMON_DATA_DIR 우선, 없으면 실행 파일 옆)
	dataDir := getEnv("OWLMON_DATA_DIR", "")
	if dataDir == "" {
		exePath, _ := os.Executable()
		// go run 시 tmp 경로 방지: 실행 파일이 tmp 폴더면 현재 디렉토리 사용
		if strings.Contains(filepath.ToSlash(exePath), "/tmp/") || strings.Contains(exePath, `\AppData\Local\Temp\`) {
			dataDir = "."
		} else {
			dataDir = filepath.Dir(exePath)
		}
	}
	configPath := filepath.Join(dataDir, "alert-config.json")
	configStore := alert.NewConfigStore(configPath)

	// PostgreSQL 연결 (설정된 경우)
	var historySaver alert.HistorySaver
	var historyStore *db.AlertHistoryStore
	pgDSN := getEnv("POSTGRES_DSN", "")
	if pgDSN != "" {
		pool, err := db.Connect(context.Background(), pgDSN)
		if err != nil {
			log.Printf("PostgreSQL 연결 실패 (알림 히스토리 비활성화): %v", err)
		} else {
			saver := db.NewHistorySaver(pool)
			historySaver = saver
			historyStore = db.NewAlertHistoryStore(pool)
		}
	} else {
		log.Println("POSTGRES_DSN 미설정 — 알림 히스토리 비활성화")
	}

	// 알림 체커 초기화
	smtpHost := getEnv("SMTP_HOST", "")
	if smtpHost != "" {
		emailCfg := &alert.EmailConfig{
			Host:     smtpHost,
			Port:     getEnv("SMTP_PORT", "587"),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     getEnv("SMTP_FROM", ""),
			To:       strings.Split(getEnv("SMTP_TO", ""), ","),
		}
		checker := alert.NewChecker(prometheusURL, emailCfg, configStore, historySaver)
		checker.Start(30 * time.Second)
	} else {
		log.Println("SMTP_HOST 미설정 — 이메일 알림 비활성화")
	}

	authHandler := handler.NewAuthHandler(username, passwordHash, jwtSecret)
	proxyHandler, err := handler.NewProxyHandler(prometheusURL)
	if err != nil {
		log.Fatalf("Prometheus 프록시 초기화 실패: %v", err)
	}
	alertHandler := handler.NewAlertHandler(configStore)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Post("/api/auth/login", authHandler.Login)
	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(jwtSecret))
		r.Handle("/api/v1/*", proxyHandler)
		r.Get("/api/alert/config", alertHandler.GetConfig)
		r.Post("/api/alert/config", alertHandler.SetConfig)
		if historyStore != nil {
			historyHandler := handler.NewHistoryHandler(historyStore)
			r.Get("/api/alert/history", historyHandler.List)
		}
	})

	srv := &http.Server{Addr: listenAddr, Handler: r}

	go func() {
		log.Printf("OWLmon 서버 시작: %s", listenAddr)
		log.Printf("Prometheus: %s", prometheusURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
