package handler

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/seongJae/owlmon/server/db"
)

// AgentKeyAuth는 에이전트 전용 엔드포인트의 X-Agent-Key 인증 미들웨어를 만든다.
// 공유 키(legacyKey)와 일치하거나, 등록된 active 에이전트의 키면 통과한다.
func AgentKeyAuth(legacyKey string, store *db.AgentStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !validAgentKey(r, legacyKey, store) {
				http.Error(w, "인증 실패 또는 비활성화된 에이전트", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// validAgentKey — 미들웨어와 LogHandler.Ingest가 공유하는 키 검증.
func validAgentKey(r *http.Request, legacyKey string, store *db.AgentStore) bool {
	key := r.Header.Get("X-Agent-Key")
	if key == "" {
		return false
	}
	// 상수시간 비교 — 응답 시간 차이로 키를 한 글자씩 추측하는 타이밍 공격 방지
	if legacyKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(legacyKey)) == 1 {
		return true
	}
	if store != nil {
		if agent, err := store.GetByKey(r.Context(), key); err == nil && agent.Status == "active" {
			go store.UpdateLastSeen(context.Background(), key)
			return true
		}
	}
	return false
}
