package handler

import (
	"log"
	"net/http"
)

// respondError는 내부 에러를 서버 로그에만 남기고, 클라이언트에는 일반화된 메시지를 보낸다.
// pgx 등 내부 에러 문자열에 테이블명/쿼리 구조가 담겨 외부로 노출되는 것을 방지한다.
func respondError(w http.ResponseWriter, status int, publicMsg string, err error) {
	if err != nil {
		log.Printf("[handler] %s: %v", publicMsg, err)
	}
	http.Error(w, publicMsg, status)
}
