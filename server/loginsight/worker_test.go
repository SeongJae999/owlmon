package loginsight

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

// === Stubs ===

type stubAnalyzer struct {
	called   int
	response Insight
	err      error
}

func (s *stubAnalyzer) Analyze(ctx context.Context, g LogGroup) (*Insight, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	ins := s.response
	if ins.SummaryKo == "" {
		ins.SummaryKo = "stub"
	}
	if ins.Severity == "" {
		ins.Severity = SeverityLow
	}
	if ins.Category == "" {
		ins.Category = CatOther
	}
	if ins.ModelName == "" {
		ins.ModelName = "stub"
	}
	ins.TemplateHash = g.TemplateHash
	ins.HostName = g.HostName
	return &ins, nil
}

type stubStore struct {
	saved   []Insight
	cached  map[string]*Insight
	touched []string
	saveErr error
}

func newStubStore() *stubStore {
	return &stubStore{cached: map[string]*Insight{}}
}

func (s *stubStore) Save(ctx context.Context, ins *Insight) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, *ins)
	return nil
}

func (s *stubStore) ByTemplate(ctx context.Context, hash string, maxAge time.Duration) (*Insight, error) {
	if v, ok := s.cached[hash]; ok {
		return v, nil
	}
	return nil, nil
}

func (s *stubStore) TouchTemplate(ctx context.Context, hash string) error {
	s.touched = append(s.touched, hash)
	return nil
}

// === 인터페이스 호환성 ===

func TestInterfacesSatisfied(t *testing.T) {
	var _ analyzerIface = (*stubAnalyzer)(nil)
	var _ storeIface = (*stubStore)(nil)
	var _ analyzerIface = (*Analyzer)(nil)
	var _ storeIface = (*Store)(nil)
}

// === rateLimit ===

func TestRateLimit_NoLimit(t *testing.T) {
	groups := []LogGroup{
		{HostName: "web1", TemplateHash: "a"},
		{HostName: "web1", TemplateHash: "b"},
		{HostName: "web1", TemplateHash: "c"},
	}
	if got := rateLimit(groups, 0, 5*time.Minute); len(got) != 3 {
		t.Errorf("zero limit should keep all, got %d", len(got))
	}
}

func TestRateLimit_PerHostScalesWithWindow(t *testing.T) {
	groups := make([]LogGroup, 15)
	for i := range groups {
		groups[i] = LogGroup{HostName: "web1", TemplateHash: strconv.Itoa(i)}
	}
	// 2 per min * 5 min window = 호스트당 10 그룹 허용
	if got := rateLimit(groups, 2, 5*time.Minute); len(got) != 10 {
		t.Errorf("expected 10 after rate limit, got %d", len(got))
	}
}

func TestRateLimit_PerHostIndependent(t *testing.T) {
	groups := []LogGroup{
		{HostName: "web1", TemplateHash: "a"},
		{HostName: "web1", TemplateHash: "b"},
		{HostName: "web1", TemplateHash: "c"}, // web1: 3, 한도 2면 c 차단
		{HostName: "web2", TemplateHash: "d"},
		{HostName: "web2", TemplateHash: "e"},
	}
	if got := rateLimit(groups, 2, 1*time.Minute); len(got) != 4 {
		t.Errorf("expected 4 (2 per host), got %d", len(got))
	}
}

// === processGroups ===

func newTestWorker(ana analyzerIface, store storeIface, budget int) *Worker {
	return &Worker{
		cfg: Config{
			CacheTTL:           24 * time.Hour,
			MaxLLMCallsPerTick: budget,
			PerHostPerMin:      100,
			Window:             5 * time.Minute,
		},
		analyzer: ana,
		store:    store,
	}
}

func TestProcessGroups_CacheHitSkipsLLM(t *testing.T) {
	ana := &stubAnalyzer{}
	store := newStubStore()
	store.cached["hit"] = &Insight{TemplateHash: "hit"}

	w := newTestWorker(ana, store, 10)
	groups := []LogGroup{
		{TemplateHash: "hit", HostName: "web1"},
		{TemplateHash: "miss", HostName: "web1"},
	}
	n, err := w.processGroups(context.Background(), groups)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 analyzed, got %d", n)
	}
	if ana.called != 1 {
		t.Errorf("expected 1 LLM call, got %d", ana.called)
	}
	if len(store.touched) != 1 || store.touched[0] != "hit" {
		t.Errorf("expected touched=[hit], got %v", store.touched)
	}
	if len(store.saved) != 1 {
		t.Errorf("expected 1 saved, got %d", len(store.saved))
	}
}

func TestProcessGroups_BudgetLimit(t *testing.T) {
	ana := &stubAnalyzer{}
	store := newStubStore()
	w := newTestWorker(ana, store, 2)

	groups := make([]LogGroup, 5)
	for i := range groups {
		groups[i] = LogGroup{TemplateHash: strconv.Itoa(i), HostName: "web1"}
	}
	n, _ := w.processGroups(context.Background(), groups)
	if n != 2 {
		t.Errorf("expected 2 (budget), got %d", n)
	}
	if ana.called != 2 {
		t.Errorf("expected 2 LLM calls, got %d", ana.called)
	}
}

func TestProcessGroups_AnalyzerErrorContinues(t *testing.T) {
	ana := &stubAnalyzer{err: errors.New("LLM down")}
	store := newStubStore()
	w := newTestWorker(ana, store, 10)

	groups := []LogGroup{
		{TemplateHash: "a", HostName: "web1"},
		{TemplateHash: "b", HostName: "web1"},
	}
	n, err := w.processGroups(context.Background(), groups)
	if err != nil {
		t.Errorf("processGroups should not return error on analyzer fail: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 analyzed on error, got %d", n)
	}
	if ana.called != 2 {
		t.Errorf("should still try all groups, got %d calls", ana.called)
	}
	if len(store.saved) != 0 {
		t.Errorf("expected 0 saved, got %d", len(store.saved))
	}
}

func TestProcessGroups_SaveErrorContinues(t *testing.T) {
	ana := &stubAnalyzer{}
	store := newStubStore()
	store.saveErr = errors.New("DB down")
	w := newTestWorker(ana, store, 10)

	groups := []LogGroup{{TemplateHash: "a", HostName: "web1"}}
	n, _ := w.processGroups(context.Background(), groups)
	if n != 0 {
		t.Errorf("save 실패 시 analyzed=0이어야 함, got %d", n)
	}
}

// === Start ===

func TestStart_DisabledNoPanic(t *testing.T) {
	w := New(Config{Enabled: false}, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Start(ctx)
}

// === Config ===

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("OWLMON_LOGINSIGHT_ENABLED", "")
	t.Setenv("OWLMON_LOGINSIGHT_INTERVAL", "")
	t.Setenv("OWLMON_LOGINSIGHT_PER_HOST_PER_MIN", "")
	t.Setenv("OWLMON_LOGINSIGHT_CACHE_TTL", "")

	cfg := LoadConfig()
	if cfg.Enabled {
		t.Errorf("default Enabled should be false")
	}
	if cfg.Interval != 5*time.Minute {
		t.Errorf("default Interval should be 5m, got %s", cfg.Interval)
	}
	if cfg.PerHostPerMin != 10 {
		t.Errorf("default PerHostPerMin should be 10, got %d", cfg.PerHostPerMin)
	}
	if cfg.CacheTTL != 24*time.Hour {
		t.Errorf("default CacheTTL should be 24h, got %s", cfg.CacheTTL)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	t.Setenv("OWLMON_LOGINSIGHT_ENABLED", "true")
	t.Setenv("OWLMON_LOGINSIGHT_INTERVAL", "10m")
	t.Setenv("OWLMON_LOGINSIGHT_PER_HOST_PER_MIN", "5")
	t.Setenv("OWLMON_LOGINSIGHT_CACHE_TTL", "12h")

	cfg := LoadConfig()
	if !cfg.Enabled {
		t.Error("Enabled not loaded from env")
	}
	if cfg.Interval != 10*time.Minute {
		t.Errorf("Interval: %s", cfg.Interval)
	}
	if cfg.PerHostPerMin != 5 {
		t.Errorf("PerHostPerMin: %d", cfg.PerHostPerMin)
	}
	if cfg.CacheTTL != 12*time.Hour {
		t.Errorf("CacheTTL: %s", cfg.CacheTTL)
	}
}

func TestLoadConfig_InvalidFallsBack(t *testing.T) {
	t.Setenv("OWLMON_LOGINSIGHT_INTERVAL", "not-a-duration")
	t.Setenv("OWLMON_LOGINSIGHT_PER_HOST_PER_MIN", "abc")

	cfg := LoadConfig()
	if cfg.Interval != 5*time.Minute {
		t.Errorf("invalid duration should fall back to 5m, got %s", cfg.Interval)
	}
	if cfg.PerHostPerMin != 10 {
		t.Errorf("invalid int should fall back to 10, got %d", cfg.PerHostPerMin)
	}
}
