package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/masking"
	"github.com/seongJae/owlmon/server/rules"
)

// SendAlertFn은 알림 발송 콜백 (Checker.SendAlert 시그니처).
type SendAlertFn func(host, category, severity, subject, body string)

// LogHandler는 로그 수집/검색/라벨링 API를 처리합니다.
type LogHandler struct {
	store      *db.LogStore
	annStore   *db.LogAnnotationStore // 라벨링 (옵션)
	agentStore *db.AgentStore         // 개별 키 검증용
	matchStore *db.LogRuleMatchStore  // 룰 매칭 기록 저장 (옵션, nil이면 기록 X)
	sendAlert  SendAlertFn            // 알림 발송 콜백 (nil이면 알림 비활성)
	legacyKey  string                 // Agent Key (env)
	rules      *rules.Engine          // 룰 매칭 엔진 (nil이면 룰 기능 비활성)
	maskOpts   masking.MaskOptions    // PII 마스킹 옵션
}

func NewLogHandler(
	store *db.LogStore,
	annStore *db.LogAnnotationStore,
	agentStore *db.AgentStore,
	legacyKey string,
	engine *rules.Engine,
	matchStore *db.LogRuleMatchStore,
	sendAlert SendAlertFn,
) *LogHandler {
	return &LogHandler{
		store:      store,
		annStore:   annStore,
		agentStore: agentStore,
		matchStore: matchStore,
		sendAlert:  sendAlert,
		legacyKey:  legacyKey,
		rules:      engine,
		maskOpts:   masking.DefaultOptions(),
	}
}

// Ingest POST /api/logs/ingest — 에이전트에서 호출 (Agent Key 인증)
func (h *LogHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	// Agent Key 인증
	key := r.Header.Get("X-Agent-Key")
	
	valid := false
	if h.legacyKey != "" && key == h.legacyKey {
		valid = true
	} else if h.agentStore != nil {
		agent, err := h.agentStore.GetByKey(r.Context(), key)
		if err == nil && agent.Status == "active" {
			valid = true
			go h.agentStore.UpdateLastSeen(context.Background(), key)
		}
	}

	if !valid {
		http.Error(w, "인증 실패 또는 비활성화된 에이전트", http.StatusUnauthorized)
		return
	}

	var req struct {
		Entries []struct {
			Timestamp string `json:"timestamp"`
			Host      string `json:"host"`
			Source    string `json:"source"`
			FilePath  string `json:"file_path"`
			Line      string `json:"line"`
			Level     string `json:"level"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	records := make([]db.LogRecord, 0, len(req.Entries))
	for _, e := range req.Entries {
		ts, _ := time.Parse(time.RFC3339, e.Timestamp)
		if ts.IsZero() {
			ts = time.Now()
		}
		// PII 마스킹 (저장 전 — DB엔 평문 개인정보 남지 않음)
		line := masking.Mask(e.Line, h.maskOpts)
		records = append(records, db.LogRecord{
			Timestamp: ts,
			Host:      e.Host,
			Source:    e.Source,
			FilePath:  e.FilePath,
			Line:      line,
			Level:     e.Level,
		})
	}

	ids, err := h.store.Ingest(r.Context(), records)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 룰 매칭 — 비동기로 평가 + 매칭 저장 + 알림 트리거
	if h.rules != nil && h.rules.RuleCount() > 0 && len(ids) == len(records) {
		go h.evaluateRulesAsync(records, ids)
	}

	w.WriteHeader(http.StatusCreated)
}

// evaluateRulesAsync는 ingest된 로그에 룰 평가 → 매칭 저장 → 알림 트리거.
func (h *LogHandler) evaluateRulesAsync(records []db.LogRecord, ids []int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 매칭 수집
	type triggerKey struct {
		ruleID int64
		host   string
	}
	var matches []db.LogRuleMatch
	triggers := make(map[triggerKey]rules.Match) // 동일 (룰,호스트) 묶어서 한 번만 알림

	for i, rec := range records {
		for _, m := range h.rules.Evaluate(rec.Line) {
			matches = append(matches, db.LogRuleMatch{
				LogID:    ids[i],
				RuleID:   m.RuleID,
				Host:     rec.Host,
				Severity: m.Severity,
			})
			triggers[triggerKey{m.RuleID, rec.Host}] = m
		}
	}
	if len(matches) == 0 {
		return
	}

	// 2. 매칭 일괄 저장
	if h.matchStore != nil {
		if err := h.matchStore.BatchInsert(ctx, matches); err != nil {
			// 매칭 저장 실패해도 알림은 계속 시도
		}
	}

	// 3. 알림 트리거 — 룰별 cooldown + threshold 평가 후 발송
	if h.sendAlert == nil {
		return
	}
	for key, m := range triggers {
		ok, _ := h.rules.ShouldAlert(ctx, key.ruleID, m.CooldownSeconds, m.ThresholdCount, m.ThresholdWindow)
		if !ok {
			continue
		}
		subject := fmt.Sprintf("[OWLmon] %s — %s", key.host, m.Name)
		var detail string
		if m.ThresholdCount > 0 && m.ThresholdWindow > 0 {
			detail = fmt.Sprintf(" (%d초 이내 %d회 이상 매칭)", m.ThresholdWindow, m.ThresholdCount)
		}
		body := fmt.Sprintf(
			"호스트: %s\n룰: %s%s\n심각도: %s\n자세히: 웹 대시보드 → 로그 룰 / 로그 뷰어에서 확인.",
			key.host, m.Name, detail, m.Severity)
		h.sendAlert(key.host, "log_rule", m.Severity, subject, body)
		_ = h.rules.RecordAlert(ctx, key.ruleID)
	}
}

// Search GET /api/logs — 로그 검색
func (h *LogHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := db.LogSearchParams{
		Host:   q.Get("host"),
		Source: q.Get("source"),
		Level:  q.Get("level"),
		Query:  q.Get("query"),
	}
	params.Limit, _ = strconv.Atoi(q.Get("limit"))
	if params.Limit <= 0 {
		params.Limit = 100
	}
	params.Offset, _ = strconv.Atoi(q.Get("offset"))

	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			params.From = t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			params.To = t
		}
	}

	result, err := h.store.Search(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Sources GET /api/logs/sources — 사용 가능한 로그 소스 목록
func (h *LogHandler) Sources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.Sources(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}

// GetByID GET /api/logs/{id} — 단일 로그 상세 조회 (라벨링 UI용)
func (h *LogHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	rec, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "로그를 찾을 수 없음", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}

// Annotate POST /api/logs/{id}/annotate — 로그에 (원인, 조치) 라벨 부여
//
// 요청 body:
//   { "category": "root_cause" | "action_taken" | "false_positive",
//     "problem": "...", "solution": "...", "alert_id": 123 }
//
// JWT 미들웨어에서 username을 추출해 annotator로 기록합니다.
func (h *LogHandler) Annotate(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}

	logID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}

	// 대상 로그 존재 확인 + log_timestamp 가져오기
	rec, err := h.store.GetByID(r.Context(), logID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "로그를 찾을 수 없음", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Category string `json:"category"`
		Problem  string `json:"problem"`
		Solution string `json:"solution"`
		AlertID  *int64 `json:"alert_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청", http.StatusBadRequest)
		return
	}
	if req.Problem == "" && req.Solution == "" {
		http.Error(w, "problem 또는 solution 중 최소 하나는 필요", http.StatusBadRequest)
		return
	}

	annotator := auth.UsernameFromContext(r.Context())
	if annotator == "" {
		annotator = "anonymous"
	}

	a := &db.LogAnnotation{
		LogID:        logID,
		LogTimestamp: rec.Timestamp,
		Annotator:    annotator,
		Category:     req.Category,
		Problem:      req.Problem,
		Solution:     req.Solution,
		AlertID:      req.AlertID,
	}
	if err := h.annStore.Insert(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(a)
}

// ListAnnotationsByLog GET /api/logs/{id}/annotations — 특정 로그의 라벨 목록
func (h *LogHandler) ListAnnotationsByLog(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}
	logID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	list, err := h.annStore.ListByLogID(r.Context(), logID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.LogAnnotation{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// ListAnnotations GET /api/logs/annotations — 전체 라벨 목록 (학습 데이터 추출용)
func (h *LogHandler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	list, total, err := h.annStore.List(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.LogAnnotation{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"items": list,
		"total": total,
	})
}

// DeleteAnnotation DELETE /api/logs/annotations/{id} — 라벨 삭제 (오타 수정 등)
func (h *LogHandler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	if h.annStore == nil {
		http.Error(w, "라벨링 기능 비활성", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 id", http.StatusBadRequest)
		return
	}
	if err := h.annStore.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
