package handler

import (
	"context"
	"net/http"

	"github.com/seongJae/owlmon/server/db"
)

// AgentKeyAuth는 에이전트 전용 엔드포인트의 X-Agent-Key 인증 미들웨어를 만든다.
// 공유 키(legacyKey)와 일치하거나, 등록된 active 에이전트의 키면 통과한다.
// LogHandler.Ingest의 검증과 동일 규칙 — 에이전트는 메트릭/로그/스펙에 같은 키를 쓴다.
func AgentKeyAuth(legacyKey string, store *db.AgentStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Agent-Key")

			valid := false
			if legacyKey != "" && key == legacyKey {
				valid = true
			} else if store != nil && key != "" {
				if agent, err := store.GetByKey(r.Context(), key); err == nil && agent.Status == "active" {
					valid = true
					go store.UpdateLastSeen(context.Background(), key)
				}
			}

			if !valid {
				http.Error(w, "인증 실패 또는 비활성화된 에이전트", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
