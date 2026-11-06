package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	usernameKey contextKey = "username"
	roleKey     contextKey = "role"
	sessionKey  contextKey = "session"

	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// UsernameFromContext는 JWTMiddleware가 주입한 username을 반환합니다.
// 토큰이 없거나 미인증 요청이면 빈 문자열.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(usernameKey).(string)
	return v
}

func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(roleKey).(string)
	return v
}

func SessionFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionKey).(string)
	return v
}

// UserCheck는 현재 역할과 세션·계정 유효 여부를 돌려준다. nil이면(DB 없는 단일 계정 모드) 항상 관리자.
type UserCheck func(ctx context.Context, username, sessionID string) (role string, active bool)

// JWTMiddleware는 Authorization 헤더의 Bearer 토큰을 검증합니다.
func JWTMiddleware(secret string, check UserCheck) func(http.Handler) http.Handler {
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

			role := RoleAdmin
			if check != nil {
				var active bool
				if role, active = check(r.Context(), claims.Username, claims.ID); !active {
					http.Error(w, "로그인이 만료되었거나 사용할 수 없는 계정입니다", http.StatusUnauthorized)
					return
				}
			}

			ctx := context.WithValue(r.Context(), usernameKey, claims.Username)
			ctx = context.WithValue(ctx, roleKey, role)
			ctx = context.WithValue(ctx, sessionKey, claims.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// 조회 전용 계정도 쓸 수 있는 POST — 상태를 바꾸지 않거나(AI 질문) 담당자가 해야 하는 일(알림 확인·내 비밀번호)
var viewerAllowedWrites = map[string]bool{
	"/api/alert/ack":     true,
	"/api/chat":          true,
	"/api/llm/explain":   true,
	"/api/llm/summary":   true,
	"/api/auth/password": true,
	"/api/auth/logout":   true,
}

// ReadOnlyGuard는 조회 전용 계정의 변경 요청을 막는다. 화면마다 권한 코드를 두지 않고 여기 한 곳에서 처리.
func ReadOnlyGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RoleFromContext(r.Context()) == RoleViewer &&
			r.Method != http.MethodGet && r.Method != http.MethodHead &&
			!viewerAllowedWrites[r.URL.Path] {
			http.Error(w, "조회 전용 계정은 변경할 수 없습니다", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RoleFromContext(r.Context()) != RoleAdmin {
			http.Error(w, "관리자만 사용할 수 있습니다", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
