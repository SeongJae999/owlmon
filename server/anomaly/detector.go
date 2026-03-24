package anomaly

import (
	"math"
	"strconv"
	"sync"
	"time"
)

// 계절성 구분: 업무시간, 야간, 주말
type TimeSlot int

const (
	SlotBusinessHours TimeSlot = iota // 평일 09~18
	SlotNight                         // 평일 야간
	SlotWeekend                       // 주말
)

const (
	windowSize   = 120 // 이동평균 윈도우 크기 (30초 간격 × 120 = 1시간)
	minSamples   = 30  // Z-score 계산에 필요한 최소 샘플 수
	zThreshold   = 3.0 // Z-score 이상치 판정 기준
	zWarning     = 2.5 // Z-score 경고 기준
)

// Anomaly는 이상탐지 결과입니다.
type Anomaly struct {
	Host     string    `json:"host"`
	Metric   string    `json:"metric"`   // cpu, memory, disk 등
	Value    float64   `json:"value"`    // 현재 값
	ZScore   float64   `json:"z_score"`  // Z-score
	Mean     float64   `json:"mean"`     // 이동평균
	StdDev   float64   `json:"std_dev"`  // 표준편차
	Severity string    `json:"severity"` // warning, critical
	Message  string    `json:"message"`
	DetectedAt time.Time `json:"detected_at"`
}

// metricKey는 호스트+메트릭+시간대별 고유 키
type metricKey struct {
	host   string
	metric string
	slot   TimeSlot
}

// metricWindow는 이동평균/표준편차 계산용 윈도우
type metricWindow struct {
	values []float64
	pos    int    // 순환 버퍼 현재 위치
	count  int    // 실제 저장된 샘플 수
	sum    float64
	sumSq  float64
}

func newMetricWindow() *metricWindow {
	return &metricWindow{
		values: make([]float64, windowSize),
	}
}

// Add는 새 값을 추가하고 이동평균/표준편차를 갱신합니다.
func (w *metricWindow) Add(val float64) {
	if w.count >= windowSize {
		// 가장 오래된 값 제거
		old := w.values[w.pos]
		w.sum -= old
		w.sumSq -= old * old
	} else {
		w.count++
	}
	w.values[w.pos] = val
	w.sum += val
	w.sumSq += val * val
	w.pos = (w.pos + 1) % windowSize
}

// Mean은 현재 이동평균을 반환합니다.
func (w *metricWindow) Mean() float64 {
	if w.count == 0 {
		return 0
	}
	return w.sum / float64(w.count)
}

// StdDev는 현재 표준편차를 반환합니다.
func (w *metricWindow) StdDev() float64 {
	if w.count < 2 {
		return 0
	}
	n := float64(w.count)
	variance := (w.sumSq / n) - (w.sum/n)*(w.sum/n)
	if variance < 0 {
		variance = 0 // 부동소수점 오차 보정
	}
	return math.Sqrt(variance)
}

// Ready는 Z-score 계산이 가능한지 반환합니다.
func (w *metricWindow) Ready() bool {
	return w.count >= minSamples
}

// Detector는 Z-score 기반 이상탐지 엔진입니다.
type Detector struct {
	mu      sync.RWMutex
	windows map[metricKey]*metricWindow
	// 최근 이상탐지 결과 (호스트별 최신만 유지)
	anomalies map[string][]Anomaly // key: host
}

func NewDetector() *Detector {
	return &Detector{
		windows:   make(map[metricKey]*metricWindow),
		anomalies: make(map[string][]Anomaly),
	}
}

// GetTimeSlot은 시간대를 계절성 슬롯으로 분류합니다.
func GetTimeSlot(t time.Time) TimeSlot {
	weekday := t.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return SlotWeekend
	}
	hour := t.Hour()
	if hour >= 9 && hour < 18 {
		return SlotBusinessHours
	}
	return SlotNight
}

// Feed는 새로운 메트릭 값을 주입하고 이상 여부를 판정합니다.
// 반환: 이상이 감지되면 Anomaly, 아니면 nil
func (d *Detector) Feed(host, metric string, value float64, ts time.Time) *Anomaly {
	slot := GetTimeSlot(ts)
	key := metricKey{host: host, metric: metric, slot: slot}

	d.mu.Lock()
	defer d.mu.Unlock()

	w, ok := d.windows[key]
	if !ok {
		w = newMetricWindow()
		d.windows[key] = w
	}

	w.Add(value)

	if !w.Ready() {
		return nil
	}

	mean := w.Mean()
	stddev := w.StdDev()
	if stddev < 0.01 {
		// 표준편차가 거의 0이면 Z-score 무의미 (값이 일정함)
		return nil
	}

	zscore := (value - mean) / stddev

	var severity string
	if math.Abs(zscore) >= zThreshold {
		severity = "critical"
	} else if math.Abs(zscore) >= zWarning {
		severity = "warning"
	} else {
		// 이상 아님 — 기존 이상 상태 해제
		d.clearAnomaly(host, metric)
		return nil
	}

	anomaly := &Anomaly{
		Host:       host,
		Metric:     metric,
		Value:      value,
		ZScore:     math.Round(zscore*100) / 100,
		Mean:       math.Round(mean*100) / 100,
		StdDev:     math.Round(stddev*100) / 100,
		Severity:   severity,
		Message:    formatAnomalyMessage(host, metric, value, zscore, mean),
		DetectedAt: ts,
	}

	d.setAnomaly(host, *anomaly)
	return anomaly
}

// GetAnomalies는 현재 감지된 이상 목록을 반환합니다.
func (d *Detector) GetAnomalies() []Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []Anomaly
	for _, anomalies := range d.anomalies {
		result = append(result, anomalies...)
	}
	return result
}

// GetHostAnomalies는 특정 호스트의 이상 목록을 반환합니다.
func (d *Detector) GetHostAnomalies(host string) []Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.anomalies[host]
}

// Stats는 현재 추적 중인 메트릭 수를 반환합니다.
func (d *Detector) Stats() (windowCount, anomalyCount int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	total := 0
	for _, a := range d.anomalies {
		total += len(a)
	}
	return len(d.windows), total
}

// InjectTestAnomaly는 테스트용 이상 데이터를 직접 주입합니다.
func (d *Detector) InjectTestAnomaly(a Anomaly) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.setAnomaly(a.Host, a)
}

// ClearAll은 모든 이상 데이터를 초기화합니다.
func (d *Detector) ClearAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anomalies = make(map[string][]Anomaly)
}

func (d *Detector) setAnomaly(host string, a Anomaly) {
	// 같은 호스트+메트릭 이상은 덮어쓰기
	existing := d.anomalies[host]
	for i, e := range existing {
		if e.Metric == a.Metric {
			existing[i] = a
			return
		}
	}
	d.anomalies[host] = append(existing, a)
}

func (d *Detector) clearAnomaly(host, metric string) {
	existing := d.anomalies[host]
	for i, e := range existing {
		if e.Metric == metric {
			d.anomalies[host] = append(existing[:i], existing[i+1:]...)
			if len(d.anomalies[host]) == 0 {
				delete(d.anomalies, host)
			}
			return
		}
	}
}

func formatAnomalyMessage(host, metric string, value, zscore, mean float64) string {
	metricName := map[string]string{
		"cpu":    "CPU",
		"memory": "메모리",
		"disk":   "디스크",
	}[metric]
	if metricName == "" {
		metricName = metric
	}

	direction := "높음"
	if zscore < 0 {
		direction = "낮음"
	}

	return metricName + " 사용률이 평소 대비 비정상적으로 " + direction +
		" (현재 " + formatFloat(value) + "%, 평균 " + formatFloat(mean) + "%, Z=" + formatFloat(zscore) + ")"
}

func formatFloat(v float64) string {
	s := math.Round(v*10) / 10
	if s == math.Trunc(s) {
		return strconv.FormatFloat(s, 'f', 0, 64)
	}
	return strconv.FormatFloat(s, 'f', 1, 64)
}
