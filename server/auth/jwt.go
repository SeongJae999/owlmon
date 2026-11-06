package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

const (
	IdleTimeout = 30 * time.Minute // 관리자 무조작 제한
	adminTTL    = 8 * time.Hour    // 공공기관 업무시간 기준
	// 상황판은 heartbeat로 계속 연장됨. 네트워크가 며칠 끊겨도 다시 로그인하지 않도록 길게 둠
	viewerTTL = 7 * 24 * time.Hour
)

func TokenTTL(role string) time.Duration {
	if role == RoleViewer {
		return viewerTTL
	}
	return adminTTL
}

// GenerateToken은 JWT 토큰을 생성합니다.
func GenerateToken(username, sessionID, secret string, ttl time.Duration) (string, error) {
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        sessionID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken은 JWT 토큰을 검증하고 Claims를 반환합니다.
func ValidateToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("유효하지 않은 토큰")
	}
	return claims, nil
}
