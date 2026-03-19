package report

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/seongJae/owlmon/server/alert"
)

// HostReport는 단일 호스트의 월간 통계입니다.
type HostReport struct {
	Host      string  `json:"host"`
	UptimePct float64 `json:"uptime_pct"`
	CPUAvg    float64 `json:"cpu_avg"`
	CPUMax    float64 `json:"cpu_max"`
	MemAvg    float64 `json:"mem_avg"`
	MemMax    float64 `json:"mem_max"`
	DiskMax   float64 `json:"disk_max"`
}

// MonthlyReport는 월간 보고서 데이터입니다.
type MonthlyReport struct {
	Year  int          `json:"year"`
	Month int          `json:"month"`
	Hosts []HostReport `json:"hosts"`
}

// Reporter는 월간 보고서를 생성하고 이메일로 발송합니다.
type Reporter struct {
	prometheusURL string
	email         *alert.EmailConfig
	configStore   alert.ConfigStorer
}

func NewReporter(prometheusURL string, email *alert.EmailConfig, configStore alert.ConfigStorer) *Reporter {
	return &Reporter{
		prometheusURL: prometheusURL,
		email:         email,
		configStore:   configStore,
	}
}

// Start는 매월 1일 09:00에 지난달 보고서를 자동 발송합니다.
func (r *Reporter) Start() {
	go func() {
		for {
			now := time.Now()
			// 다음달 1일 09:00 계산
			next := time.Date(now.Year(), now.Month()+1, 1, 9, 0, 0, 0, now.Location())
			log.Printf("월간 보고서 다음 발송: %s", next.Format("2006-01-02 15:04:05"))
			time.Sleep(time.Until(next))

			// 지난달 보고서 발송
			prev := next.AddDate(0, -1, 0)
			if err := r.SendReport(prev.Year(), prev.Month()); err != nil {
				log.Printf("월간 보고서 자동 발송 실패: %v", err)
			}
		}
	}()
	log.Println("월간 보고서 스케줄러 시작 (매월 1일 09:00)")
}

// Generate는 지정된 년월의 보고서 데이터를 생성합니다.
func (r *Reporter) Generate(year int, month time.Month) (*MonthlyReport, error) {
	end := time.Date(year, month+1, 1, 0, 0, 0, 0, time.Local).Add(-time.Second)
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	days := int(end.Sub(start).Hours()/24) + 1
	duration := fmt.Sprintf("%dd", days)

	hosts, err := r.labelValues("host_name")
	if err != nil {
		return nil, fmt.Errorf("호스트 목록 조회 실패: %w", err)
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("등록된 호스트가 없습니다")
	}

	var hostReports []HostReport
	for _, host := range hosts {
		hr := HostReport{Host: host}

		q := func(promql string) float64 {
			v, _ := r.instantQuery(promql, end)
			return v
		}

		hr.CPUAvg = q(fmt.Sprintf(`avg_over_time(system_cpu_usage_percent{host_name="%s"}[%s])`, host, duration))
		hr.CPUMax = q(fmt.Sprintf(`max_over_time(system_cpu_usage_percent{host_name="%s"}[%s])`, host, duration))
		hr.MemAvg = q(fmt.Sprintf(`avg_over_time(system_memory_usage_percent{host_name="%s"}[%s])`, host, duration))
		hr.MemMax = q(fmt.Sprintf(`max_over_time(system_memory_usage_percent{host_name="%s"}[%s])`, host, duration))
		hr.DiskMax = q(fmt.Sprintf(`max(max_over_time(system_disk_usage_percent{host_name="%s"}[%s]))`, host, duration))

		// 가동률: 실제 샘플 수 / 예상 샘플 수 (30초 간격)
		count := q(fmt.Sprintf(`count_over_time(system_cpu_usage_percent{host_name="%s"}[%s])`, host, duration))
		expectedSamples := float64(days * 24 * 120) // 30초 간격 = 분당 2회 = 시간당 120회
		if expectedSamples > 0 && count > 0 {
			hr.UptimePct = min(count/expectedSamples*100, 100)
		}

		hostReports = append(hostReports, hr)
	}

	return &MonthlyReport{Year: year, Month: int(month), Hosts: hostReports}, nil
}

// SendReport는 보고서를 생성하여 이메일로 발송합니다.
func (r *Reporter) SendReport(year int, month time.Month) error {
	rep, err := r.Generate(year, month)
	if err != nil {
		return err
	}

	cfg := r.configStore.Get()
	to := cfg.Recipients
	if len(to) == 0 {
		to = r.email.To
	}
	if len(to) == 0 {
		return fmt.Errorf("수신자가 설정되지 않았습니다")
	}

	subject := fmt.Sprintf("%d년 %d월 시스템 월간 보고서", year, int(month))
	body := formatText(rep)
	return r.email.SendTo(to, subject, body)
}

// formatText는 보고서를 텍스트 형식으로 포맷합니다.
func formatText(rep *MonthlyReport) string {
	daysInMonth := time.Date(rep.Year, time.Month(rep.Month)+1, 0, 0, 0, 0, 0, time.Local).Day()
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("=== %d년 %d월 시스템 월간 보고서 ===\n", rep.Year, rep.Month))
	sb.WriteString(fmt.Sprintf("보고 기간: %d-%02d-01 ~ %d-%02d-%02d\n\n", rep.Year, rep.Month, rep.Year, rep.Month, daysInMonth))

	for _, h := range rep.Hosts {
		uptimeBar := progressBar(h.UptimePct, 20)
		cpuBar := progressBar(h.CPUAvg, 20)
		memBar := progressBar(h.MemAvg, 20)

		sb.WriteString(fmt.Sprintf("■ %s\n", h.Host))
		sb.WriteString(fmt.Sprintf("  가동률  %s %.1f%%\n", uptimeBar, h.UptimePct))
		sb.WriteString(fmt.Sprintf("  CPU 평균 %s %.1f%% (최대 %.1f%%)\n", cpuBar, h.CPUAvg, h.CPUMax))
		sb.WriteString(fmt.Sprintf("  메모리   %s %.1f%% (최대 %.1f%%)\n", memBar, h.MemAvg, h.MemMax))
		sb.WriteString(fmt.Sprintf("  디스크 최대: %.1f%%\n", h.DiskMax))
		sb.WriteString("\n")
	}

	sb.WriteString("OWLmon 모니터링 시스템")
	return sb.String()
}

// progressBar는 ASCII 진행 바를 생성합니다.
func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// --- Prometheus 조회 헬퍼 ---

func (r *Reporter) instantQuery(promql string, at time.Time) (float64, error) {
	params := url.Values{}
	params.Set("query", promql)
	params.Set("time", strconv.FormatInt(at.Unix(), 10))

	resp, err := http.Get(r.prometheusURL + "/api/v1/query?" + params.Encode())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Result []struct {
				Value [2]interface{} `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	if len(result.Data.Result) == 0 {
		return 0, nil
	}
	valStr, _ := result.Data.Result[0].Value[1].(string)
	return strconv.ParseFloat(valStr, 64)
}

func (r *Reporter) labelValues(label string) ([]string, error) {
	resp, err := http.Get(r.prometheusURL + "/api/v1/label/" + label + "/values")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
