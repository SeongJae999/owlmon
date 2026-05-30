package loginsight

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// analyzerIface — Worker가 사용하는 분석기 인터페이스.
// 실제로는 *Analyzer, 테스트에서는 stub.
type analyzerIface interface {
	Analyze(ctx context.Context, g LogGroup) (*Insight, error)
}

// storeIface — Worker가 사용하는 저장소 인터페이스.
type storeIface interface {
	Save(ctx context.Context, ins *Insight) error
	ByTemplate(ctx context.Context, hash string, maxAge time.Duration) (*Insight, error)
	TouchTemplate(ctx context.Context, hash string) error
}

// Worker — log_insights 백그라운드 워커.
// 매 Interval마다 Tick()을 실행해 최근 Window의 ERROR/WARN 로그를 분석.
type Worker struct {
	cfg      Config
	pool     *pgxpool.Pool
	analyzer analyzerIface
	store    storeIface
}

// New — Worker 생성. pool/analyzer/store가 nil이면 Start가 no-op.
func New(cfg Config, pool *pgxpool.Pool, analyzer analyzerIface, store storeIface) *Worker {
	return &Worker{cfg: cfg, pool: pool, analyzer: analyzer, store: store}
}

// Start — 백그라운드 고루틴으로 워커 시작.
// 비활성/의존성 부재 시 즉시 반환 (정상 동작).
// ctx 취소 시 종료.
func (w *Worker) Start(ctx context.Context) {
	if !w.cfg.Enabled || w.pool == nil || w.analyzer == nil || w.store == nil {
		log.Printf("[loginsight] 비활성 (enabled=%v pool=%v analyzer=%v store=%v)",
			w.cfg.Enabled, w.pool != nil, w.analyzer != nil, w.store != nil)
		return
	}
	if err := EnsureTables(ctx, w.pool); err != nil {
		log.Printf("[loginsight] EnsureTables 실패: %v", err)
		return
	}
	log.Printf("[loginsight] 워커 시작 (interval=%s window=%s cacheTTL=%s)",
		w.cfg.Interval, w.cfg.Window, w.cfg.CacheTTL)
	go w.loop(ctx)
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	// 첫 실행은 30초 지연 (서버 부팅 직후 부하 분산)
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	w.runTick(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[loginsight] 워커 종료")
			return
		case <-ticker.C:
			w.runTick(ctx)
		}
	}
}

func (w *Worker) runTick(ctx context.Context) {
	start := time.Now()
	n, err := w.Tick(ctx)
	if err != nil {
		log.Printf("[loginsight] tick 실패: %v (소요 %s)", err, time.Since(start))
		return
	}
	if n > 0 {
		log.Printf("[loginsight] tick 완료 — %d건 분석 (소요 %s)", n, time.Since(start))
	}
}

// Tick — 1회 사이클. fetch → cluster → processGroups.
// 외부 호출 가능 (테스트/디버그).
func (w *Worker) Tick(ctx context.Context) (int, error) {
	lines, err := w.fetchRecentLogs(ctx)
	if err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		return 0, nil
	}
	groups := Cluster(lines, w.cfg.SamplesPerGroup)
	return w.processGroups(ctx, groups)
}

// processGroups — 그룹 리스트를 받아 4중 필터(rate limit·캐시·예산) 적용하며 분석.
// 외부 fetch와 분리해 단위 테스트가 쉽도록 함.
func (w *Worker) processGroups(ctx context.Context, groups []LogGroup) (int, error) {
	groups = rateLimit(groups, w.cfg.PerHostPerMin, w.cfg.Window)
	budget := w.cfg.MaxLLMCallsPerTick
	if budget <= 0 {
		budget = len(groups)
	}
	analyzed := 0
	for _, g := range groups {
		if budget <= 0 {
			break
		}
		cached, _ := w.store.ByTemplate(ctx, g.TemplateHash, w.cfg.CacheTTL)
		if cached != nil {
			_ = w.store.TouchTemplate(ctx, g.TemplateHash)
			continue
		}
		ins, err := w.analyzer.Analyze(ctx, g)
		if err != nil {
			log.Printf("[loginsight] Analyze 실패 (host=%s hash=%s): %v",
				g.HostName, g.TemplateHash, err)
			continue
		}
		if err := w.store.Save(ctx, ins); err != nil {
			log.Printf("[loginsight] Save 실패: %v", err)
			continue
		}
		analyzed++
		budget--
	}
	return analyzed, nil
}

// fetchRecentLogs — logs 테이블에서 Window 내 ERROR/WARN 등 로그 조회.
// 자체 SQL (db 패키지 변경 회피).
func (w *Worker) fetchRecentLogs(ctx context.Context) ([]LogLine, error) {
	cutoff := time.Now().Add(-w.cfg.Window)
	levels := make([]string, len(w.cfg.SeverityFilter))
	for i, s := range w.cfg.SeverityFilter {
		levels[i] = strings.ToLower(s)
	}
	rows, err := w.pool.Query(ctx, `
		SELECT id, host, line
		FROM logs
		WHERE timestamp >= $1
		  AND LOWER(level) = ANY($2)
		ORDER BY timestamp ASC
		LIMIT 5000`,
		cutoff, levels)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogLine
	for rows.Next() {
		var l LogLine
		if err := rows.Scan(&l.ID, &l.HostName, &l.Message); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// rateLimit — Layer 2 필터: 호스트당 (perHostPerMin × windowMin) 그룹만 통과.
func rateLimit(groups []LogGroup, perHostPerMin int, window time.Duration) []LogGroup {
	if perHostPerMin <= 0 {
		return groups
	}
	windowMin := int(window / time.Minute)
	if windowMin < 1 {
		windowMin = 1
	}
	perHostMax := perHostPerMin * windowMin

	seen := map[string]int{}
	out := make([]LogGroup, 0, len(groups))
	for _, g := range groups {
		if seen[g.HostName] >= perHostMax {
			continue
		}
		seen[g.HostName]++
		out = append(out, g)
	}
	return out
}
