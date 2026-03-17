package main

import (
	"log"
	"net/http"
	"os"


	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/handler"
)

func main() {
	// 환경변수로 계정 관리 (납품 시 설치 스크립트에서 설정)
	username     := getEnv("OWLMON_USERNAME", "admin")
	passwordHash := getEnv("OWLMON_PASSWORD_HASH", "") // bcrypt 해시
	jwtSecret    := getEnv("OWLMON_JWT_SECRET", "change-this-secret-in-production")
	prometheusURL := getEnv("OWLMON_PROMETHEUS_URL", "http://localhost:9090")
	listenAddr   := getEnv("OWLMON_LISTEN", ":8080")

	if passwordHash == "" {
		log.Fatal("OWLMON_PASSWORD_HASH 환경변수가 설정되지 않았습니다.\n" +
			"다음 명령어로 해시를 생성하세요:\n" +
			"  go run ./cmd/hashpw <비밀번호>")
	}

	// 핸들러 초기화
	authHandler := handler.NewAuthHandler(username, passwordHash, jwtSecret)
	proxyHandler, err := handler.NewProxyHandler(prometheusURL)
	if err != nil {
		log.Fatalf("Prometheus 프록시 초기화 실패: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// 로그인 (인증 불필요)
	r.Post("/api/auth/login", authHandler.Login)

	// Prometheus 프록시 (JWT 필요) - 경로 그대로 Prometheus에 전달
	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(jwtSecret))
		r.Handle("/api/v1/*", proxyHandler)
	})

	log.Printf("OWLmon 서버 시작: %s", listenAddr)
	log.Printf("Prometheus: %s", prometheusURL)
	if err := http.ListenAndServe(listenAddr, r); err != nil {
		log.Fatalf("서버 시작 실패: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
