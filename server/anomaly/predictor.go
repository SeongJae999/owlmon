package anomaly

import (
	"math"
	"strconv"
	"sync"
	"time"
)

const (
	predictionWindowSize = 720 // 6시간 (30초 간격 × 720)
	minPredictionSamples = 120 // 최소 1시간 데이터
	diskFullThreshold    = 95  // 디스크 "가득 참" 기준 (%)
)

// DiskPrediction은 디스크 고갈 예측 결과입니다.
type DiskPrediction struct {
	Host       string  `json:"host"`
	Mountpoint string  `json:"mountpoint"`
	Current    float64 `json:"current"`      // 현재 사용률 (%)
	Slope      float64 `json:"slope"`        // 시간당 변화율 (%/h)
	DaysLeft   float64 `json:"days_left"`    // 고갈까지 남은 일수 (-1이면 감소 추세)
	R2         float64 `json:"r2"`           // 결정계수 (모델 신뢰도)
	Message    string  `json:"message"`
}

type diskKey struct {
	host       string
	mountpoint string
}

// diskSample은 시계열 데이터 포인트
type diskSample struct {
	timestamp float64 // Unix 초
	value     float64 // 사용률 (%)
}

type diskWindow struct {
	samples []diskSample
	pos     int
	count   int
}

func newDiskWindow() *diskWindow {
	return &diskWindow{
		samples: make([]diskSample, predictionWindowSize),
	}
}

func (w *diskWindow) Add(ts time.Time, val float64) {
	s := diskSample{timestamp: float64(ts.Unix()), value: val}
	w.samples[w.pos] = s
	w.pos = (w.pos + 1) % predictionWindowSize
	if w.count < predictionWindowSize {
		w.count++
	}
}

func (w *diskWindow) Ready() bool {
	return w.count >= minPredictionSamples
}

// getSamples는 시간순 정렬된 현재 샘플을 반환합니다.
func (w *diskWindow) getSamples() []diskSample {
	result := make([]diskSample, w.count)
	start := w.pos - w.count
	if start < 0 {
		start += predictionWindowSize
	}
	for i := 0; i < w.count; i++ {
		idx := (start + i) % predictionWindowSize
		result[i] = w.samples[idx]
	}
	return result
}

// Predictor는 선형회귀 기반 디스크 고갈 예측기입니다.
type Predictor struct {
	mu          sync.RWMutex
	windows     map[diskKey]*diskWindow
	predictions map[diskKey]*DiskPrediction
}

func NewPredictor() *Predictor {
	return &Predictor{
		windows:     make(map[diskKey]*diskWindow),
		predictions: make(map[diskKey]*DiskPrediction),
	}
}

// Feed는 디스크 사용률 데이터를 주입하고 예측을 갱신합니다.
func (p *Predictor) Feed(host, mountpoint string, value float64, ts time.Time) *DiskPrediction {
	key := diskKey{host: host, mountpoint: mountpoint}

	p.mu.Lock()
	defer p.mu.Unlock()

	w, ok := p.windows[key]
	if !ok {
		w = newDiskWindow()
		p.windows[key] = w
	}

	w.Add(ts, value)

	if !w.Ready() {
		return nil
	}

	samples := w.getSamples()
	slope, intercept, r2 := linearRegression(samples)

	pred := &DiskPrediction{
		Host:       host,
		Mountpoint: mountpoint,
		Current:    math.Round(value*10) / 10,
		Slope:      math.Round(slope*3600*1000) / 1000, // %/시간
		R2:         math.Round(r2*1000) / 1000,
	}

	if slope <= 0 {
		// 감소 추세: 고갈 없음
		pred.DaysLeft = -1
		pred.Message = host + " " + mountpoint + ": 디스크 사용량 안정/감소 추세"
	} else {
		// 고갈까지 남은 시간 (초)
		lastTs := samples[len(samples)-1].timestamp
		timeToFull := (diskFullThreshold - (slope*lastTs + intercept)) / slope
		daysLeft := timeToFull / 86400
		if daysLeft < 0 {
			daysLeft = 0
		}
		pred.DaysLeft = math.Round(daysLeft*10) / 10
		pred.Message = formatPredictionMessage(host, mountpoint, value, daysLeft)
	}

	// R² < 0.5이면 신뢰도 낮음 — 저장만 하고 알림은 하지 않음
	p.predictions[key] = pred
	return pred
}

// GetPredictions는 모든 디스크 예측 결과를 반환합니다.
func (p *Predictor) GetPredictions() []DiskPrediction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []DiskPrediction
	for _, pred := range p.predictions {
		result = append(result, *pred)
	}
	return result
}

// GetCriticalPredictions는 N일 이내 고갈 예상 디스크를 반환합니다.
func (p *Predictor) GetCriticalPredictions(withinDays float64) []DiskPrediction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []DiskPrediction
	for _, pred := range p.predictions {
		if pred.DaysLeft >= 0 && pred.DaysLeft <= withinDays && pred.R2 >= 0.5 {
			result = append(result, *pred)
		}
	}
	return result
}

// InjectTestPrediction은 테스트용 디스크 예측 데이터를 직접 주입합니다.
func (p *Predictor) InjectTestPrediction(pred DiskPrediction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := diskKey{host: pred.Host, mountpoint: pred.Mountpoint}
	p.predictions[key] = &pred
}

// ClearAll은 모든 예측 데이터를 초기화합니다.
func (p *Predictor) ClearAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.predictions = make(map[diskKey]*DiskPrediction)
}

// linearRegression은 최소제곱법으로 선형회귀를 수행합니다.
// 반환: slope, intercept, R²
func linearRegression(samples []diskSample) (slope, intercept, r2 float64) {
	n := float64(len(samples))
	if n < 2 {
		return 0, 0, 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for _, s := range samples {
		sumX += s.timestamp
		sumY += s.value
		sumXY += s.timestamp * s.value
		sumX2 += s.timestamp * s.timestamp
		sumY2 += s.value * s.value
	}

	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return 0, sumY / n, 0
	}

	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n

	// R² (결정계수)
	meanY := sumY / n
	var ssTot, ssRes float64
	for _, s := range samples {
		predicted := slope*s.timestamp + intercept
		ssRes += (s.value - predicted) * (s.value - predicted)
		ssTot += (s.value - meanY) * (s.value - meanY)
	}
	if ssTot == 0 {
		r2 = 1
	} else {
		r2 = 1 - ssRes/ssTot
	}

	return slope, intercept, r2
}

func formatPredictionMessage(host, mountpoint string, current, daysLeft float64) string {
	if daysLeft <= 1 {
		return host + " " + mountpoint + ": 현재 " + formatPct(current) + "%, 24시간 내 디스크 부족 예상!"
	} else if daysLeft <= 7 {
		return host + " " + mountpoint + ": 현재 " + formatPct(current) + "%, 약 " + formatDays(daysLeft) + "일 후 디스크 부족 예상"
	} else if daysLeft <= 30 {
		return host + " " + mountpoint + ": 현재 " + formatPct(current) + "%, 약 " + formatDays(daysLeft) + "일 후 디스크 부족 예상"
	}
	return host + " " + mountpoint + ": 현재 추세면 " + formatDays(daysLeft) + "일 후 디스크 부족"
}

func formatPct(v float64) string {
	r := math.Round(v*10) / 10
	if r == math.Trunc(r) {
		return formatDays(r) // 정수면 소수점 없이
	}
	return formatDays(r)
}

func formatDays(v float64) string {
	if v == math.Trunc(v) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
