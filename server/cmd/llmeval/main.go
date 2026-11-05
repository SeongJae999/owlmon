package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/loginsight"
)

type evalCase struct {
	ID        string `json:"id"`
	Log       string `json:"log"`
	WantSev   string `json:"want_sev"`
	WantHuman bool   `json:"want_human"`
	Note      string `json:"note"`
	HistDays  int    `json:"hist_days"`
	HistN     int    `json:"hist_n"` // >0이면 [패턴 이력] 주입
}

type modelOut struct {
	Severity   string `json:"severity"`
	NeedsHuman bool   `json:"needs_human"`
	SummaryKo  string `json:"summary_ko"`
}

func main() {
	path := "../scripts/llm-eval/eval-set.jsonl"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	provider := &llm.OllamaProvider{
		URL:   envOr("OWLMON_OLLAMA_URL", "http://localhost:11434"),
		Model: envOr("OWLMON_OLLAMA_MODEL", "gemma4:12b"),
		JSON:  os.Getenv("LLMEVAL_PLAIN") == "", // 운영 워커와 같은 설정으로 평가
	}
	fmt.Printf("모델: %s\n평가셋: %s\n\n", provider.Name(), path)

	f, err := os.Open(path)
	if err != nil {
		fmt.Println("평가셋 열기 실패:", err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Printf("%-20s %-12s %-12s %s\n", "ID", "기대(sev/h)", "실제(sev/h)", "판정")
	fmt.Println(strings.Repeat("─", 78))

	var total, sevHit, humanHit, bothHit, dropped int
	var fails []string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c evalCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		total++

		group := loginsight.LogGroup{
			HostName:        "evalhost",
			Count:           1,
			TemplateExample: c.Log,
			SampleLines:     []string{c.Log},
		}
		if c.HistN > 0 {
			group.FirstSeenAt = time.Now().Add(-time.Duration(c.HistDays)*24*time.Hour - time.Hour)
			group.PriorAnalyses = c.HistN
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		raw, err := provider.Complete(ctx, loginsight.SystemPromptLogCluster, loginsight.BuildUserMsg(group))
		cancel()

		var got modelOut
		var ins loginsight.Insight
		if err == nil {
			_ = json.Unmarshal([]byte(extractJSON(raw)), &got)
			_ = json.Unmarshal([]byte(extractJSON(raw)), &ins)
		}
		// 운영 워커는 Validate 실패 시 인사이트를 버린다 — 판단이 맞아도 화면엔 안 나감
		vErr := ins.Validate()
		if vErr != nil {
			dropped++
		}

		sevOK := got.Severity == c.WantSev
		humanOK := got.NeedsHuman == c.WantHuman
		if sevOK {
			sevHit++
		}
		if humanOK {
			humanHit++
		}
		verdict := "✗"
		if sevOK && humanOK {
			bothHit++
			verdict = "✓"
		} else {
			fails = append(fails, fmt.Sprintf("  [%s] %s → 기대 %s/%v, 실제 %s/%v",
				c.ID, c.Note, c.WantSev, c.WantHuman, got.Severity, got.NeedsHuman))
		}

		if vErr != nil {
			verdict += " ⚠검증탈락(" + vErr.Error() + ")"
		}
		fmt.Printf("%-20s %-12s %-12s %s  %s\n",
			c.ID,
			fmt.Sprintf("%s/%v", c.WantSev, c.WantHuman),
			fmt.Sprintf("%s/%v", got.Severity, got.NeedsHuman),
			verdict, got.SummaryKo)
	}

	fmt.Println(strings.Repeat("─", 78))
	pct := func(n int) float64 { return float64(n) / float64(total) * 100 }
	fmt.Printf("\n총 %d건\n", total)
	fmt.Printf("  심각도 일치:     %d/%d (%.0f%%)\n", sevHit, total, pct(sevHit))
	fmt.Printf("  대응필요 일치:   %d/%d (%.0f%%)\n", humanHit, total, pct(humanHit))
	fmt.Printf("  둘 다 일치:      %d/%d (%.0f%%)  ← 종합 정확도\n", bothHit, total, pct(bothHit))
	fmt.Printf("  운영 검증 탈락:  %d/%d (Validate 실패 → 운영에서 버려짐)\n", dropped, total)
	if len(fails) > 0 {
		fmt.Println("\n불일치:")
		for _, s := range fails {
			fmt.Println(s)
		}
	}
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
