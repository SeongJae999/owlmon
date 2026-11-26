package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/db"
)

type agentNameKey struct{}

// AgentName — 인증된 에이전트 키에 묶인 장비 이름. 공용 키(전환 설정)로 들어왔으면 "".
// 에이전트가 보낸 host 값 대신 이 이름을 써야 다른 장비 행세를 못 한다.
func AgentName(ctx context.Context) string {
	name, _ := ctx.Value(agentNameKey{}).(string)
	return name
}

// AgentKeyAuth는 에이전트 전용 엔드포인트의 키 인증 미들웨어를 만든다.
// legacyKey는 OWLMON_LEGACY_SHARED_KEY=true일 때만 비어 있지 않다.
func AgentKeyAuth(legacyKey string, store *db.AgentStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name, ok := agentIdentity(r, legacyKey, store)
			if !ok {
				http.Error(w, "인증 실패 또는 폐기된 에이전트", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), agentNameKey{}, name)))
		})
	}
}

// AgentAuthCheck GET /api/agent/auth — nginx auth_request가 메트릭(gRPC)마다 부른다.
// 통과하면 장비 이름을 X-Owlmon-Host로 돌려주고, nginx가 그 값을 Collector에 넘겨 host.name을 덮어쓴다.
// nginx 내부 전용 — 밖에서 부르면 키가 맞는지·어느 장비 키인지 알려 주는 창구가 되므로,
// nginx 컨테이너에서 온 요청만 받는다(8080 직접 접속 차단). 바깥 /api/ 경로는 nginx.conf가 따로 404로 막는다
func AgentAuthCheck(legacyKey string, store *db.AgentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !audit.FromTrustedProxy(r) {
			http.NotFound(w, r)
			return
		}
		name, ok := agentIdentity(r, legacyKey, store)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if name != "" {
			w.Header().Set("X-Owlmon-Host", name)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// AgentBinaryAuth — /api/agent/binary·latest용. 설치 스크립트는 아직 키가 없으므로 유효한 등록 토큰도 받는다(소진하지 않음).
func AgentBinaryAuth(legacyKey string, store *db.AgentStore) func(http.Handler) http.Handler {
	keyAuth := AgentKeyAuth(legacyKey, store)
	return func(next http.Handler) http.Handler {
		withKey := keyAuth(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if t := r.Header.Get("X-Enroll-Token"); t != "" && store != nil && store.TokenUsable(r.Context(), t) {
				next.ServeHTTP(w, r)
				return
			}
			withKey.ServeHTTP(w, r)
		})
	}
}

// agentIdentity — 미들웨어·nginx 확인·LogHandler.Ingest가 공유하는 키 검증.
// 키는 X-Agent-Key(HTTP) 또는 Authorization: Bearer(OTLP gRPC)로 온다.
func agentIdentity(r *http.Request, legacyKey string, store *db.AgentStore) (string, bool) {
	key := r.Header.Get("X-Agent-Key")
	if key == "" {
		key, _ = strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if key == "" {
		return "", false
	}
	if store != nil {
		if agent, err := store.GetByKey(r.Context(), key); err == nil {
			if agent.Status != "active" {
				return "", false
			}
			go store.UpdateLastSeen(context.Background(), agent.ID)
			return agent.Name, true
		}
	}
	// 상수시간 비교 — 응답 시간 차이로 키를 한 글자씩 추측하는 타이밍 공격 방지
	if legacyKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(legacyKey)) == 1 {
		return "", true
	}
	return "", false
}
