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
	"github.com/seongJae/owlmon/server/report"
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
	jwtSecret := getEnv("OWLMON_JWT_SECRET", "")
	if jwtSecret == "" || jwtSecret == "change-this-secret-in-production" {
		b := make([]byte, 32)
		rand.Read(b)
		jwtSecret = hex.EncodeToString(b)
		log.Printf("⚠️  OWLMON_JWT_SECRET 미설정 — 임시 시크릿 생성됨 (재시작 시 로그인 세션 초기화됨)")
		log.Printf("   .env에 다음을 추가하세요: OWLMON_JWT_SECRET=%s", jwtSecret)
	}
	prometheusURL := getEnv("OWLMON_PROMETHEUS_URL", "http://localhost:9090")
	listenAddr := getEnv("OWLMON_LISTEN", ":8080")

	if passwordHash == "" {
		log.Fatal("OWLMON_PASSWORD_HASH 환경변수가 설정되지 않았습니다.\n" +
			"다음 명령어로 해시를 생성하세요:\n" +
			"  go run ./cmd/hashpw <비밀번호>")
	}

	// PostgreSQL 연결 (설정된 경우)
	var configStore alert.ConfigStorer
	var historySaver alert.HistorySaver
	var historyStore *db.AlertHistoryStore
	pgDSN := getEnv("POSTGRES_DSN", "")
	if pgDSN != "" {
		pool, err := db.Connect(context.Background(), pgDSN)
		if err != nil {
			log.Printf("PostgreSQL 연결 실패: %v", err)
		} else {
			log.Println("PostgreSQL 연결 성공")
			configStore = db.NewAlertConfigStore(pool)
			historySaver = db.NewHistorySaver(pool)
			historyStore = db.NewAlertHistoryStore(pool)
		}
	}
	// PostgreSQL 미연결 시 파일 기반 폴백
	if configStore == nil {
		log.Println("POSTGRES_DSN 미설정 — 알림 설정/히스토리를 파일로 저장")
		dataDir := getEnv("OWLMON_DATA_DIR", "")
		if dataDir == "" {
			exePath, _ := os.Executable()
			if strings.Contains(filepath.ToSlash(exePath), "/tmp/") || strings.Contains(exePath, `\AppData\Local\Temp\`) {
				dataDir = "."
			} else {
				dataDir = filepath.Dir(exePath)
			}
		}
		configStore = alert.NewConfigStore(filepath.Join(dataDir, "alert-config.json"))
	}

	// 알림 체커 초기화
	smtpHost := getEnv("SMTP_HOST", "")
	var emailCfg *alert.EmailConfig
	if smtpHost != "" {
		emailCfg = &alert.EmailConfig{
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
	statusHandler := handler.NewStatusHandler(prometheusURL, configStore)

	// 월간 보고서
	var reportHandler *handler.ReportHandler
	if emailCfg != nil {
		reporter := report.NewReporter(prometheusURL, emailCfg, configStore)
		reporter.Start()
		reportHandler = handler.NewReportHandler(reporter)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Post("/api/auth/login", authHandler.Login)
	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(jwtSecret))
		r.Handle("/api/v1/*", proxyHandler)
		r.Get("/api/alert/config", alertHandler.GetConfig)
		r.Post("/api/alert/config", alertHandler.SetConfig)
		r.Get("/api/alert/status", statusHandler.GetStatus)
		if historyStore != nil {
			historyHandler := handler.NewHistoryHandler(historyStore)
			r.Get("/api/alert/history", historyHandler.List)
		}
		if reportHandler != nil {
			r.Get("/api/report/preview", reportHandler.Preview)
			r.Post("/api/report/send", reportHandler.Send)
		}
	})

	tlsCert := getEnv("OWLMON_TLS_CERT", "")
	tlsKey := getEnv("OWLMON_TLS_KEY", "")

	srv := &http.Server{Addr: listenAddr, Handler: r}

	go func() {
		log.Printf("OWLmon 서버 시작: %s", listenAddr)
		log.Printf("Prometheus: %s", prometheusURL)
		var err error
		if tlsCert != "" && tlsKey != "" {
			cert, loadErr := tls.LoadX509KeyPair(tlsCert, tlsKey)
			if loadErr != nil {
				log.Fatalf("TLS 인증서 로드 실패: %v", loadErr)
			}
			srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
			log.Printf("HTTPS 활성화 (인증서: %s)", tlsCert)
			err = srv.ListenAndServeTLS("", "")
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
