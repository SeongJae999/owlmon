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
	defaultWindowSize = 120 // 기본 이동평균 윈도우 크기 (1시간)
	minSamples        = 30  // Z-score 계산에 필요한 최소 샘플 수
	zThreshold        = 3.0 // Z-score 이상치 판정 기준
	zWarning          = 2.5 // Z-score 경고 기준
)

// 절대값 필터 — z-score만 보면 11% CPU 변동도 critical로 잡힘
// 실제 운영자 입장에서 의미 있는 절대값 수준 이상일 때만 anomaly 알람 발사
var minAbsoluteForAnomaly = map[string]float64{
	"cpu":    50, // 50% 미만은 z-score 높아도 노이즈
	"memory": 60,
	"disk":   70,
}

// 진짜 critical 절대값 임계치 — 이 이상이고 z-score도 critical 이면 critical 알람
var criticalAbsoluteForAnomaly = map[string]float64{
	"cpu":    80,
	"memory": 90,
	"disk":   85,
}

// Anomaly는 이상탐지 결과입니다.
type Anomaly struct {
	Host       string    `json:"host"`
	Metric     string    `json:"metric"`   // cpu, memory, disk 등
	Value      float64   `json:"value"`    // 현재 값
	ZScore     float64   `json:"z_score"`  // Z-score
	Mean       float64   `json:"mean"`     // 이동평균
	StdDev     float64   `json:"std_dev"`  // 표준편차
	Severity   string    `json:"severity"` // warning, critical
	Message    string    `json:"message"`
	DetectedAt time.Time `json:"detected_at"`
}

// metricKey는 호스트+메트릭+시간대별 고유 키
type metricKey struct {
	host   string
	metric string
	slot   TimeSlot
}

// metricWindow는 이동평균/표준편차 계산용 윈도우 (Welford's Algorithm 기반 슬라이딩 윈도우)
type metricWindow struct {
	values []float64
	pos    int
	count  int
	size   int
	mean   float64
	m2     float64 // Sum of squares of differences from the mean
}

func newMetricWindow(size int) *metricWindow {
	if size <= 0 {
		size = defaultWindowSize
	}
	return &metricWindow{
		values: make([]float64, size),
		size:   size,
	}
}

// Add는 새 값을 추가하고 Welford's 알고리즘을 사용하여 평균/분산을 안정적으로 갱신합니다.
func (w *metricWindow) Add(val float64) {
	if w.count < w.size {
		w.count++
		delta := val - w.mean
		w.mean += delta / float64(w.count)
		delta2 := val - w.mean
		w.m2 += delta * delta2
	} else {
		oldVal := w.values[w.pos]
		deltaOld := oldVal - w.mean
		w.mean -= deltaOld / float64(w.size)
		w.m2 -= deltaOld * (oldVal - w.mean)

		deltaNew := val - w.mean
		w.mean += deltaNew / float64(w.size)
		w.m2 += deltaNew * (val - w.mean)

		if w.m2 < 0 {
			w.m2 = 0
		}
	}
	w.values[w.pos] = val
	w.pos = (w.pos + 1) % w.size
}

// Mean은 현재 이동평균을 반환합니다.
func (w *metricWindow) Mean() float64 {
	return w.mean
}

// StdDev는 현재 표준편차를 반환합니다.
func (w *metricWindow) StdDev() float64 {
	if w.count < 2 {
		return 0
	}
	return math.Sqrt(w.m2 / float64(w.count-1))
}

// Ready는 Z-score 계산이 가능한지 반환합니다.
func (w *metricWindow) Ready() bool {
	return w.count >= minSamples
}

// Detector는 Z-score 기반 이상탐지 엔진입니다.
type Detector struct {
	mu      sync.RWMutex
	windows map[metricKey]*metricWindow
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

var metricWindowSizes = map[string]int{
	"cpu":    60,
	"memory": 120,
	"disk":   240,
}

// Feed는 새로운 메트릭 값을 주입하고 이상 여부를 판정합니다.
func (d *Detector) Feed(host, metric string, value float64, ts time.Time) *Anomaly {
	slot := GetTimeSlot(ts)
	key := metricKey{host: host, metric: metric, slot: slot}

	d.mu.Lock()
	defer d.mu.Unlock()

	w, ok := d.windows[key]
	if !ok {
		size := metricWindowSizes[metric]
		w = newMetricWindow(size)
		d.windows[key] = w
	}

	w.Add(value)

	if !w.Ready() {
		return nil
	}

	mean := w.Mean()
	stddev := w.StdDev()
	if stddev < 0.01 {
		return nil
	}

	// 절대값 필터 — 미미한 수준이면 z-score 높아도 무시 (alert fatigue 방지)
	// 예: CPU 11% → 평소보다 z=3.4로 튀어도 진짜 위험 X
	if minAbs, ok := minAbsoluteForAnomaly[metric]; ok && value < minAbs {
		d.clearAnomaly(host, metric)
		return nil
	}

	zscore := (value - mean) / stddev

	// severity 결정: z-score + 절대값 둘 다 위험해야 critical
	// (z 임계치 넘어도 절대값이 낮으면 warning)
	absoluteCritical := false
	if critAbs, ok := criticalAbsoluteForAnomaly[metric]; ok && value >= critAbs {
		absoluteCritical = true
	}

	var severity string
	if math.Abs(zscore) >= zThreshold && absoluteCritical {
		severity = "critical"
	} else if math.Abs(zscore) >= zWarning {
		severity = "warning"
	} else {
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

func (d *Detector) GetAnomalies() []Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var result []Anomaly
	for _, anomalies := range d.anomalies {
		result = append(result, anomalies...)
	}
	return result
}

func (d *Detector) GetHostAnomalies(host string) []Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.anomalies[host]
}

func (d *Detector) Stats() (windowCount, anomalyCount int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	total := 0
	for _, a := range d.anomalies {
		total += len(a)
	}
	return len(d.windows), total
}

func (d *Detector) InjectTestAnomaly(a Anomaly) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.setAnomaly(a.Host, a)
}

func (d *Detector) ClearAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anomalies = make(map[string][]Anomaly)
}

func (d *Detector) setAnomaly(host string, a Anomaly) {
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
