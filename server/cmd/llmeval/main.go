package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/loginsight"
	"github.com/seongJae/owlmon/server/masking"
)

// sevSet — "high" 또는 ["medium","high"] 모두 허용 (전문가끼리도 갈리는 케이스는 범위로 채점)
type sevSet []string

func (s *sevSet) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*s = sevSet{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

func (s sevSet) has(v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

type evalCase struct {
	ID           string   `json:"id"`
	Tags         []string `json:"tags"`
	Split        string   `json:"split"`
	Label        string   `json:"label"`
	Source       string   `json:"source"`
	Note         string   `json:"note"`
	Host         string   `json:"host"`
	Log          string   `json:"log"`
	Lines        []string `json:"lines"`
	Count        int      `json:"count"`
	HostCtx      string   `json:"host_ctx"`
	Context      string   `json:"context"` // 자산 정보처럼 운영 프롬프트에 아직 없는 맥락 — 모델의 맥락 활용 능력을 미리 측정
	HistDays     int      `json:"hist_days"`
	HistN        int      `json:"hist_n"`
	WantSev      sevSet   `json:"want_sev"`
	WantHuman    *bool    `json:"want_human"` // 생략하면 채점 안 함 (대응 필요 여부도 갈리는 케이스)
	WantCategory string   `json:"want_category"`
	MustMention  []string `json:"must_mention"` // 항목마다 "a|b" 중 하나라도 요약·원인·조치에 있어야 함
	MustNot      []string `json:"must_not"`
	file         string
}

type runResult struct {
	Severity   string   `json:"severity"`
	NeedsHuman bool     `json:"needs_human"`
	Category   string   `json:"category"`
	Summary    string   `json:"summary_ko"`
	RootCause  string   `json:"root_cause_ko"`
	Action     string   `json:"action_ko"`
	LatencyMs  int64    `json:"latency_ms"`
	Err        string   `json:"err,omitempty"`
	FormatOK   bool     `json:"format_ok"`
	SevOK      bool     `json:"sev_ok"`
	HumanOK    bool     `json:"human_ok"`
	CatOK      bool     `json:"cat_ok"`
	Missing    []string `json:"missing,omitempty"`
	Violations []string `json:"violations,omitempty"`
	Pass       bool     `json:"pass"`
}

type caseResult struct {
	ID         string      `json:"id"`
	File       string      `json:"file"`
	Tags       []string    `json:"tags"`
	Split      string      `json:"split"`
	Label      string      `json:"label"`
	Runs       []runResult `json:"runs"`
	PassRate   float64     `json:"pass_rate"`
	Consistent bool        `json:"consistent"`
	HasCat     bool        `json:"has_category"`
	HasMention bool        `json:"has_mention"`
}

type report struct {
	Model      string       `json:"model"`
	PromptHash string       `json:"prompt_hash"`
	At         time.Time    `json:"at"`
	Split      string       `json:"split"`
	Repeat     int          `json:"repeat"`
	Cases      []caseResult `json:"cases"`
}

func main() {
	set := flag.String("set", "../scripts/llm-eval/eval-set*.jsonl", "평가셋 파일 (glob, 쉼표 구분)")
	split := flag.String("split", "all", "all | dev | holdout")
	repeat := flag.Int("repeat", 1, "케이스당 반복 횟수 (안정성 측정)")
	model := flag.String("model", envOr("OWLMON_OLLAMA_MODEL", "gemma4:12b"), "Ollama 모델")
	out := flag.String("out", "", "결과 JSON 저장 경로 (비우면 ../scripts/llm-eval/results/<시각>-<모델>.json)")
	compare := flag.String("compare", "", "두 결과 비교: 이전.json,이후.json")
	rescore := flag.String("rescore", "", "저장된 결과에 현재 정답지를 다시 적용 (모델 재호출 없음)")
	flag.Parse()

	if *repeat < 1 {
		fail("-repeat는 1 이상이어야 합니다")
	}
	if *rescore != "" {
		rescoreReport(*rescore, *set)
		return
	}

	if *compare != "" {
		files := strings.Split(*compare, ",")
		if len(files) != 2 {
			fail("-compare는 파일 2개가 필요합니다")
		}
		compareReports(load(files[0]), load(files[1]))
		return
	}
	if flag.NArg() > 0 {
		*set = strings.Join(flag.Args(), ",")
	}

	cases := loadCases(*set, *split)
	if len(cases) == 0 {
		fail("평가할 케이스가 없습니다: " + *set)
	}
	provider := &llm.OllamaProvider{
		URL:   envOr("OWLMON_OLLAMA_URL", "http://localhost:11434"),
		Model: *model,
		JSON:  os.Getenv("LLMEVAL_PLAIN") == "", // 운영 워커와 같은 설정으로 평가
	}
	h := sha256.Sum256([]byte(loginsight.SystemPromptLogCluster))
	rep := report{Model: provider.Name(), PromptHash: hex.EncodeToString(h[:4]), At: time.Now(), Split: *split, Repeat: *repeat}
	fmt.Printf("모델 %s · 프롬프트 %s · 케이스 %d건 × %d회 (split=%s)\n\n", rep.Model, rep.PromptHash, len(cases), *repeat, *split)

	for _, c := range cases {
		cr := caseResult{ID: c.ID, File: c.file, Tags: c.Tags, Split: c.Split, Label: c.Label, Consistent: true,
			HasCat: c.WantCategory != "", HasMention: len(c.MustMention) > 0}
		msg := buildUserMsg(c)
		passed := 0
		for i := 0; i < *repeat; i++ {
			r := runOnce(provider, msg, c)
			if r.Pass {
				passed++
			}
			if i > 0 && r.Severity != cr.Runs[0].Severity {
				cr.Consistent = false
			}
			cr.Runs = append(cr.Runs, r)
		}
		cr.PassRate = float64(passed) / float64(*repeat)
		rep.Cases = append(rep.Cases, cr)
		r := cr.Runs[0]
		fmt.Printf("%-26s %-15s %-8s %d/%d  %s\n", c.ID, strings.Join(c.WantSev, "|"), r.Severity, passed, *repeat, r.Summary)
	}

	printSummary(rep)
	path := *out
	if path == "" {
		path = fmt.Sprintf("../scripts/llm-eval/results/%s-%s.json", rep.At.Format("20060102-1504"), strings.NewReplacer(":", "_", "/", "_").Replace(*model))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(path, b, 0o644); err == nil {
			fmt.Println("\n결과 저장:", path)
		}
	}
}

// buildUserMsg — 운영 analyzer와 같은 순서: BuildUserMsg → 마스킹 → 호스트 상태 블록
func buildUserMsg(c evalCase) string {
	lines := c.Lines
	if len(lines) == 0 {
		lines = []string{c.Log}
	}
	host := c.Host
	if host == "" {
		host = "evalhost"
	}
	count := c.Count
	if count == 0 {
		count = len(lines)
	}
	g := loginsight.LogGroup{
		HostName:        host,
		Count:           count,
		TemplateExample: loginsight.Templatize(lines[0]),
		SampleLines:     lines,
	}
	if c.HistN > 0 {
		g.FirstSeenAt = time.Now().Add(-time.Duration(c.HistDays)*24*time.Hour - time.Hour)
		g.PriorAnalyses = c.HistN
	}
	msg := masking.Mask(loginsight.BuildUserMsg(g), masking.DefaultOptions())
	hostCtx := c.HostCtx
	if hostCtx == "" {
		hostCtx = "[현재 호스트 상태]\n- 사용률: 데이터 없음\n" // 운영도 수치가 없으면 이 블록을 붙인다
	}
	msg += "\n\n" + hostCtx
	if c.Context != "" {
		msg += "\n" + c.Context
	}
	return msg
}

func runOnce(p *llm.OllamaProvider, msg string, c evalCase) runResult {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	start := time.Now()
	raw, err := p.Complete(ctx, loginsight.SystemPromptLogCluster, msg)
	r := runResult{LatencyMs: time.Since(start).Milliseconds()}
	if err != nil {
		r.Err = err.Error()
		return r
	}
	var ins loginsight.Insight
	if json.Unmarshal([]byte(extractJSON(raw)), &ins) != nil {
		r.Err = "JSON 파싱 실패"
		return r
	}
	r.Severity, r.NeedsHuman, r.Category = string(ins.Severity), ins.NeedsHuman, string(ins.Category)
	r.Summary, r.RootCause, r.Action = ins.SummaryKo, ins.RootCauseKo, ins.ActionKo
	// 운영 워커는 Validate 실패 시 인사이트를 버린다 — 판단이 맞아도 화면엔 안 나감
	if err := ins.Validate(); err != nil {
		r.Err = "검증 탈락: " + err.Error()
	} else {
		r.FormatOK = true
	}
	score(&r, c)
	return r
}

// score — 정답지 비교만 분리 (정답지를 고친 뒤 -rescore로 모델 재호출 없이 다시 채점)
func score(r *runResult, c evalCase) {
	r.Missing, r.Violations = nil, nil
	r.SevOK = c.WantSev.has(r.Severity)
	r.HumanOK = c.WantHuman == nil || r.NeedsHuman == *c.WantHuman
	r.CatOK = c.WantCategory == "" || c.WantCategory == r.Category
	text := strings.ToLower(r.Summary + " " + r.RootCause + " " + r.Action)
	for _, m := range c.MustMention {
		if !containsAny(text, m) {
			r.Missing = append(r.Missing, m)
		}
	}
	for _, m := range c.MustNot {
		if containsAny(text, m) {
			r.Violations = append(r.Violations, m)
		}
	}
	r.Pass = r.FormatOK && r.SevOK && r.HumanOK && r.CatOK && len(r.Missing) == 0 && len(r.Violations) == 0
}

func rescoreReport(path, set string) {
	rep := load(path)
	byID := map[string]evalCase{}
	for _, c := range loadCases(set, "all") {
		byID[c.ID] = c
	}
	kept := rep.Cases[:0]
	for _, cr := range rep.Cases {
		c, ok := byID[cr.ID]
		if !ok {
			continue // 정답지에서 빠진 케이스는 결과에서도 제외
		}
		cr.Tags, cr.Split, cr.Label = c.Tags, c.Split, c.Label
		cr.HasCat, cr.HasMention = c.WantCategory != "", len(c.MustMention) > 0
		passed := 0
		for i := range cr.Runs {
			if r := &cr.Runs[i]; r.Err == "" || r.Severity != "" {
				// 검증 규칙이 바뀌었을 수 있어 형식도 다시 판정
				ins := loginsight.Insight{Severity: loginsight.Severity(r.Severity), Category: loginsight.Category(r.Category), SummaryKo: r.Summary}
				if err := ins.Validate(); err != nil {
					r.FormatOK, r.Err = false, "검증 탈락: "+err.Error()
				} else {
					r.FormatOK, r.Err, r.Category = true, "", string(ins.Category)
				}
				score(r, c)
			}
			passed += b2i(cr.Runs[i].Pass)
		}
		cr.PassRate = float64(passed) / float64(len(cr.Runs))
		kept = append(kept, cr)
	}
	rep.Cases = kept
	printSummary(rep)
	b, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fail("저장 실패: " + err.Error())
	}
	fmt.Println("\n재채점 저장:", path)
}

func containsAny(text, alts string) bool {
	for _, a := range strings.Split(alts, "|") {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" && strings.Contains(text, a) {
			return true
		}
	}
	return false
}

type metrics struct {
	runs, format, sev, human, cat, catN, mention, mentionN, violations, pass int
	consistent, cases                                                        int
	latencies                                                                []int64
}

func collect(cases []caseResult) metrics {
	var m metrics
	for _, c := range cases {
		m.cases++
		if c.Consistent {
			m.consistent++
		}
		for _, r := range c.Runs {
			m.runs++
			m.latencies = append(m.latencies, r.LatencyMs)
			m.format += b2i(r.FormatOK)
			m.sev += b2i(r.SevOK)
			m.human += b2i(r.HumanOK)
			if c.HasCat {
				m.catN++
				m.cat += b2i(r.CatOK)
			}
			if c.HasMention {
				m.mentionN++
				m.mention += b2i(len(r.Missing) == 0)
			}
			m.violations += len(r.Violations)
			m.pass += b2i(r.Pass)
		}
	}
	return m
}

func printSummary(rep report) {
	m := collect(rep.Cases)
	fmt.Println(strings.Repeat("─", 90))
	printMetrics("전체", m)

	fmt.Println("\n라벨별 통과율")
	for _, label := range []string{"확정", "잠정"} {
		var sub []caseResult
		for _, c := range rep.Cases {
			if c.Label == label {
				sub = append(sub, c)
			}
		}
		if len(sub) > 0 {
			sm := collect(sub)
			fmt.Printf("  %-6s %3d건  %s\n", label, len(sub), pct(sm.pass, sm.runs))
		}
	}

	fmt.Println("\n유형별 통과율")
	byTag := map[string][]caseResult{}
	for _, c := range rep.Cases {
		for _, t := range c.Tags {
			byTag[t] = append(byTag[t], c)
		}
	}
	tags := make([]string, 0, len(byTag))
	for t := range byTag {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	for _, t := range tags {
		sm := collect(byTag[t])
		fmt.Printf("  %-12s %3d건  %s\n", t, len(byTag[t]), pct(sm.pass, sm.runs))
	}

	fmt.Println("\n불통과 상세 (첫 실패 회차)")
	for _, c := range rep.Cases {
		for _, r := range c.Runs {
			if r.Pass {
				continue
			}
			var why []string
			if !r.FormatOK {
				why = append(why, "형식("+r.Err+")")
			}
			if !r.SevOK {
				why = append(why, "심각도="+r.Severity)
			}
			if !r.HumanOK {
				why = append(why, fmt.Sprintf("대응필요=%v", r.NeedsHuman))
			}
			if !r.CatOK {
				why = append(why, "분류="+r.Category)
			}
			if len(r.Missing) > 0 {
				why = append(why, "언급누락="+strings.Join(r.Missing, ","))
			}
			if len(r.Violations) > 0 {
				why = append(why, "금지표현="+strings.Join(r.Violations, ","))
			}
			fmt.Printf("  [%s] %s\n", c.ID, strings.Join(why, " · "))
			break
		}
	}
}

func printMetrics(title string, m metrics) {
	fmt.Printf("%s: 케이스 %d건 × 실행 %d회\n", title, m.cases, m.runs)
	fmt.Printf("  종합 통과        %s\n", pct(m.pass, m.runs))
	fmt.Printf("  형식(JSON·검증)  %s\n", pct(m.format, m.runs))
	fmt.Printf("  심각도(허용범위) %s\n", pct(m.sev, m.runs))
	fmt.Printf("  대응필요         %s\n", pct(m.human, m.runs))
	if m.catN > 0 {
		fmt.Printf("  분류             %s\n", pct(m.cat, m.catN))
	}
	if m.mentionN > 0 {
		fmt.Printf("  반드시 언급      %s\n", pct(m.mention, m.mentionN))
	}
	fmt.Printf("  금지 표현 위반   %d건\n", m.violations)
	fmt.Printf("  반복 일치(심각도) %s\n", pct(m.consistent, m.cases))
	p50, p95 := percentile(m.latencies, 50), percentile(m.latencies, 95)
	fmt.Printf("  지연 중앙값 %.1fs · p95 %.1fs\n", float64(p50)/1000, float64(p95)/1000)
}

func compareReports(a, b report) {
	fmt.Printf("이전: %s (프롬프트 %s, %s)\n이후: %s (프롬프트 %s, %s)\n\n",
		a.Model, a.PromptHash, a.At.Format("01-02 15:04"), b.Model, b.PromptHash, b.At.Format("01-02 15:04"))
	ma, mb := collect(a.Cases), collect(b.Cases)
	row := func(name string, x, xn, y, yn int) {
		fmt.Printf("  %-16s %8s → %8s\n", name, pct(x, xn), pct(y, yn))
	}
	row("종합 통과", ma.pass, ma.runs, mb.pass, mb.runs)
	row("심각도", ma.sev, ma.runs, mb.sev, mb.runs)
	row("대응필요", ma.human, ma.runs, mb.human, mb.runs)
	row("반복 일치", ma.consistent, ma.cases, mb.consistent, mb.cases)
	fmt.Printf("  %-16s %7.1fs → %7.1fs\n", "지연 중앙값", float64(percentile(ma.latencies, 50))/1000, float64(percentile(mb.latencies, 50))/1000)

	before := map[string]caseResult{}
	for _, c := range a.Cases {
		before[c.ID] = c
	}
	var better, worse []string
	for _, c := range b.Cases {
		old, ok := before[c.ID]
		if !ok {
			continue
		}
		d := fmt.Sprintf("  [%s] %.0f%% → %.0f%% (%s → %s)", c.ID, old.PassRate*100, c.PassRate*100, old.Runs[0].Severity, c.Runs[0].Severity)
		switch {
		case c.PassRate > old.PassRate:
			better = append(better, d)
		case c.PassRate < old.PassRate:
			worse = append(worse, d)
		}
	}
	fmt.Printf("\n개선 %d건\n%s\n회귀 %d건\n%s\n", len(better), strings.Join(better, "\n"), len(worse), strings.Join(worse, "\n"))
}

func loadCases(set, split string) []evalCase {
	var files []string
	var cases []evalCase
	for _, pat := range strings.Split(set, ",") {
		m, _ := filepath.Glob(strings.TrimSpace(pat))
		files = append(files, m...)
	}
	sort.Strings(files)
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			fail("평가셋 열기 실패: " + err.Error())
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for n := 1; sc.Scan(); n++ {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var c evalCase
			if err := json.Unmarshal([]byte(line), &c); err != nil {
				fail(fmt.Sprintf("%s:%d 형식 오류: %v", f, n, err))
			}
			c.file = filepath.Base(f)
			if c.Split == "" {
				c.Split = "dev"
			}
			if c.Label == "" {
				c.Label = "확정"
				if strings.HasPrefix(c.Note, "[잠정]") {
					c.Label = "잠정"
				}
			}
			if split == "all" || split == c.Split {
				cases = append(cases, c)
			}
		}
		fh.Close()
	}
	return cases
}

func load(path string) report {
	b, err := os.ReadFile(path)
	if err != nil {
		fail("결과 파일 읽기 실패: " + err.Error())
	}
	var r report
	if err := json.Unmarshal(b, &r); err != nil {
		fail("결과 파일 형식 오류: " + err.Error())
	}
	return r
}

func percentile(xs []int64, p int) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[(len(s)-1)*p/100]
}

func pct(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", n, d, float64(n)/float64(d)*100)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// extractJSON — LLM 응답에서 첫 '{' ~ 마지막 '}' 추출 (코드펜스·잡설 제거).
func extractJSON(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return ""
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
