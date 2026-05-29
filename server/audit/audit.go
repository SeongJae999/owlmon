// Package audit는 ISMS-P "변경 감사 추적" 의무용 audit log 기록.
//
// 사용:
//   // 명시적 기록 (변경 전/후 명확히 알 때)
//   audit.Log(ctx, audit.Entry{
//       Action: "rule.update", TargetType: "rule", TargetID: ruleID,
//       Details: map[string]any{"before": old, "after": new},
//   })
//
//   // HTTP 미들웨어 자동 기록 (변경 메서드만)
//   r.Use(audit.Middleware(store))
package audit

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/auth"
)

// Store는 audit log 저장소.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Entry는 단일 감사 이벤트.
type Entry struct {
	Actor      string
	IP         string
	Action     string
	TargetType string
	TargetID   string
	Details    map[string]any
	Result     string // success / failure / unauthorized
	UserAgent  string
}

// Save는 entry를 audit_log 테이블에 저장 — 실패해도 본 요청은 계속 (best-effort).
func (s *Store) Save(ctx context.Context, e Entry) {
	if s == nil || s.pool == nil {
		return
	}
	if e.Result == "" {
		e.Result = "success"
	}
	var detailsJSON []byte
	if len(e.Details) > 0 {
		detailsJSON, _ = json.Marshal(e.Details)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_log (actor, ip, action, target_type, target_id, details, result, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.Actor, e.IP, e.Action, e.TargetType, e.TargetID, detailsJSON, e.Result, e.UserAgent,
	)
	if err != nil {
		log.Printf("[audit] 저장 실패: %v (action=%s)", err, e.Action)
	}
}

// ──── Context 기반 헬퍼 (handler 안에서 호출) ────

type ctxKey struct{}

var auditCtxKey = ctxKey{}

// FromContext는 미들웨어가 주입한 audit info를 꺼낸다.
func FromContext(ctx context.Context) (*Info, bool) {
	v, ok := ctx.Value(auditCtxKey).(*Info)
	return v, ok
}

// Info는 요청 단위로 미들웨어가 채우는 메타.
type Info struct {
	Actor     string
	IP        string
	UserAgent string
	Store     *Store
	// Action / Target은 handler에서 추가 설정
	Action     string
	TargetType string
	TargetID   string
	Details    map[string]any
	Skip       bool // true 면 자동 기록 안 함 (예: 조회 API)
}

// Log는 명시적 감사 기록 — handler 안에서 호출.
func Log(ctx context.Context, action, targetType, targetID string, details map[string]any) {
	info, ok := FromContext(ctx)
	if !ok || info == nil || info.Store == nil {
		return
	}
	info.Store.Save(ctx, Entry{
		Actor:      info.Actor,
		IP:         info.IP,
		UserAgent:  info.UserAgent,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    details,
	})
}

// LogFailure는 인증/권한/유효성 실패도 기록 — 보안 감사용.
func LogFailure(ctx context.Context, action, reason string) {
	info, ok := FromContext(ctx)
	if !ok || info == nil || info.Store == nil {
		return
	}
	info.Store.Save(ctx, Entry{
		Actor:     info.Actor,
		IP:        info.IP,
		UserAgent: info.UserAgent,
		Action:    action,
		Result:    "failure",
		Details:   map[string]any{"reason": reason},
	})
}

// ──── HTTP 미들웨어 ────

// Middleware는 모든 요청에 Info를 주입한다.
// 변경 API (POST/PUT/PATCH/DELETE)는 별도 처리하지 않고, handler에서 audit.Log() 호출하는 패턴 권장.
func Middleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := &Info{
				Actor:     extractActor(r),
				IP:        extractIP(r),
				UserAgent: r.UserAgent(),
				Store:     store,
			}
			ctx := context.WithValue(r.Context(), auditCtxKey, info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractActor(r *http.Request) string {
	// JWT claim에서 사용자명 추출 — auth 패키지의 헬퍼 활용
	if username := auth.UsernameFromContext(r.Context()); username != "" {
		return username
	}
	return "anonymous"
}

func extractIP(r *http.Request) string {
	// X-Forwarded-For 우선 (proxy 뒤에 있을 때)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// 첫 번째 IP만
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// 직접 연결 — host:port에서 port 제거
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

// ──── 조회 ────

type SearchFilter struct {
	Actor      string
	Action     string
	TargetType string
	From       string // RFC3339
	To         string
	Limit      int
}

// ListEntry는 조회 응답용.
type ListEntry struct {
	ID         int64           `json:"id"`
	TS         string          `json:"ts"`
	Actor      string          `json:"actor"`
	IP         string          `json:"ip"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Details    json.RawMessage `json:"details,omitempty"`
	Result     string          `json:"result"`
	UserAgent  string          `json:"user_agent,omitempty"`
}

func (s *Store) Search(ctx context.Context, f SearchFilter) ([]ListEntry, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 500
	}
	q := `SELECT id, ts, actor, COALESCE(ip,''), action, COALESCE(target_type,''),
	             COALESCE(target_id,''), details, result, COALESCE(user_agent,'')
	      FROM audit_log WHERE 1=1`
	args := []any{}
	n := 1
	if f.Actor != "" {
		q += " AND actor = $" + itoa(n)
		args = append(args, f.Actor)
		n++
	}
	if f.Action != "" {
		q += " AND action = $" + itoa(n)
		args = append(args, f.Action)
		n++
	}
	if f.TargetType != "" {
		q += " AND target_type = $" + itoa(n)
		args = append(args, f.TargetType)
		n++
	}
	if f.From != "" {
		q += " AND ts >= $" + itoa(n)
		args = append(args, f.From)
		n++
	}
	if f.To != "" {
		q += " AND ts <= $" + itoa(n)
		args = append(args, f.To)
		n++
	}
	q += " ORDER BY ts DESC LIMIT $" + itoa(n)
	args = append(args, f.Limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ListEntry
	for rows.Next() {
		var e ListEntry
		var ts time.Time
		var details []byte
		if err := rows.Scan(&e.ID, &ts, &e.Actor, &e.IP, &e.Action, &e.TargetType, &e.TargetID, &details, &e.Result, &e.UserAgent); err != nil {
			return nil, err
		}
		e.TS = ts.Format(time.RFC3339)
		if len(details) > 0 {
			e.Details = details
		}
		out = append(out, e)
	}
	return out, nil
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
