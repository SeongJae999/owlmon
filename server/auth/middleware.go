package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const usernameKey contextKey = "username"

// JWTMiddleware는 Authorization 헤더의 Bearer 토큰을 검증합니다.
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "인증이 필요합니다", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := ValidateToken(tokenStr, secret)
			if err != nil {
				http.Error(w, "유효하지 않은 토큰입니다", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), usernameKey, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
