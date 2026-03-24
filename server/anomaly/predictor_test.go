package anomaly

import (
	"math"
	"testing"
	"time"
)

// --- 선형회귀 테스트 ---

func TestLinearRegression_완전한_직선(t *testing.T) {
	// y = 2x + 10 (완벽한 직선 → R² = 1)
	samples := make([]diskSample, 100)
	for i := range 100 {
		x := float64(i)
		samples[i] = diskSample{timestamp: x, value: 2*x + 10}
	}

	slope, intercept, r2 := linearRegression(samples)

	if math.Abs(slope-2) > 0.001 {
		t.Errorf("기울기 = %f, 기대값 = 2", slope)
	}
	if math.Abs(intercept-10) > 0.001 {
		t.Errorf("절편 = %f, 기대값 = 10", intercept)
	}
	if math.Abs(r2-1) > 0.001 {
		t.Errorf("R² = %f, 기대값 = 1", r2)
	}
}

func TestLinearRegression_수평선(t *testing.T) {
	// 값이 일정 → 기울기 0
	samples := make([]diskSample, 50)
	for i := range 50 {
		samples[i] = diskSample{timestamp: float64(i * 30), value: 50}
	}

	slope, _, r2 := linearRegression(samples)

	if math.Abs(slope) > 0.001 {
		t.Errorf("기울기 = %f, 기대값 ≈ 0", slope)
	}
	// R² = 1 (완벽한 예측, 분산 0)
	if math.Abs(r2-1) > 0.001 {
		t.Errorf("R² = %f, 기대값 = 1", r2)
	}
}

func TestLinearRegression_샘플_부족(t *testing.T) {
	samples := []diskSample{{timestamp: 1, value: 50}}
	slope, _, _ := linearRegression(samples)
	if slope != 0 {
		t.Errorf("샘플 1개에서 기울기 = %f, 기대값 = 0", slope)
	}
}

// --- Predictor 통합 테스트 ---

func TestPredictor_증가추세_고갈예측(t *testing.T) {
	p := NewPredictor()
	// 시간당 1%씩 증가: 60%에서 시작 → 35시간 후 95%
	ts := time.Date(2026, 3, 24, 0, 0, 0, 0, time.Local)

	var lastPred *DiskPrediction
	for i := range minPredictionSamples + 50 {
		val := 60.0 + float64(i)*30.0/3600.0 // 30초 간격, 시간당 1% 증가
		pred := p.Feed("server-01", "/", val, ts)
		if pred != nil {
			lastPred = pred
		}
		ts = ts.Add(30 * time.Second)
	}

	if lastPred == nil {
		t.Fatal("예측 결과가 없음")
	}
	if lastPred.DaysLeft < 0 {
		t.Fatal("증가 추세인데 DaysLeft < 0")
	}
	if lastPred.Slope <= 0 {
		t.Errorf("기울기 = %f, 양수여야 함", lastPred.Slope)
	}
	if lastPred.R2 < 0.9 {
		t.Errorf("R² = %f, 직선 데이터에서 0.9 이상이어야 함", lastPred.R2)
	}
	// 약 35시간 = 1.5일 근처 예상
	if lastPred.DaysLeft > 5 {
		t.Errorf("DaysLeft = %f, 2일 이내여야 함", lastPred.DaysLeft)
	}
}

func TestPredictor_감소추세_고갈없음(t *testing.T) {
	p := NewPredictor()
	ts := time.Date(2026, 3, 24, 0, 0, 0, 0, time.Local)

	for i := range minPredictionSamples + 50 {
		val := 80.0 - float64(i)*30.0/3600.0 // 시간당 1% 감소
		p.Feed("server-01", "/data", val, ts)
		ts = ts.Add(30 * time.Second)
	}

	preds := p.GetPredictions()
	if len(preds) == 0 {
		t.Fatal("예측 결과가 없음")
	}
	for _, pred := range preds {
		if pred.DaysLeft >= 0 {
			t.Errorf("감소 추세에서 DaysLeft = %f, -1이어야 함", pred.DaysLeft)
		}
	}
}

func TestPredictor_안정추세_알림없음(t *testing.T) {
	p := NewPredictor()
	ts := time.Date(2026, 3, 24, 0, 0, 0, 0, time.Local)

	for i := range minPredictionSamples + 50 {
		val := 40.0 + float64(i%10)*0.1 // 거의 일정
		p.Feed("server-01", "/", val, ts)
		ts = ts.Add(30 * time.Second)
	}

	critical := p.GetCriticalPredictions(7)
	if len(critical) != 0 {
		t.Errorf("안정 추세에서 긴급 예측 %d건", len(critical))
	}
}

func TestPredictor_GetCriticalPredictions_R2필터(t *testing.T) {
	p := NewPredictor()
	ts := time.Date(2026, 3, 24, 0, 0, 0, 0, time.Local)

	// 노이즈가 심한 데이터 → R² 낮음
	for i := range minPredictionSamples + 50 {
		val := 70.0
		if i%2 == 0 {
			val = 90.0 // 극심한 변동
		}
		p.Feed("server-01", "/noisy", val, ts)
		ts = ts.Add(30 * time.Second)
	}

	// R² < 0.5인 경우 GetCriticalPredictions에서 필터링
	critical := p.GetCriticalPredictions(30)
	for _, pred := range critical {
		if pred.R2 < 0.5 {
			t.Errorf("R² = %f인 예측이 긴급 목록에 포함됨", pred.R2)
		}
	}
}

func TestPredictor_여러_마운트포인트(t *testing.T) {
	p := NewPredictor()
	ts := time.Date(2026, 3, 24, 0, 0, 0, 0, time.Local)

	for i := range minPredictionSamples + 10 {
		p.Feed("server-01", "/", 50.0+float64(i)*0.01, ts)
		p.Feed("server-01", "/data", 70.0+float64(i)*0.01, ts)
		ts = ts.Add(30 * time.Second)
	}

	preds := p.GetPredictions()
	if len(preds) != 2 {
		t.Errorf("예측 수 = %d, 기대값 = 2", len(preds))
	}
}

// --- 디스크 윈도우 테스트 ---

func TestDiskWindow_Ready(t *testing.T) {
	w := newDiskWindow()
	ts := time.Now()

	for range minPredictionSamples - 1 {
		w.Add(ts, 50)
		ts = ts.Add(30 * time.Second)
	}
	if w.Ready() {
		t.Error("minPredictionSamples-1에서 Ready()가 true")
	}
	w.Add(ts, 50)
	if !w.Ready() {
		t.Error("minPredictionSamples에서 Ready()가 false")
	}
}

func TestDiskWindow_순환버퍼_순서(t *testing.T) {
	w := newDiskWindow()
	ts := time.Now()

	// predictionWindowSize + 10개 추가
	for i := range predictionWindowSize + 10 {
		w.Add(ts, float64(i))
		ts = ts.Add(30 * time.Second)
	}

	samples := w.getSamples()
	if len(samples) != predictionWindowSize {
		t.Fatalf("샘플 수 = %d, 기대값 = %d", len(samples), predictionWindowSize)
	}
	// 첫 번째 값은 10 (0~9가 밀려남)
	if samples[0].value != 10 {
		t.Errorf("첫 번째 값 = %f, 기대값 = 10", samples[0].value)
	}
	// 마지막 값
	last := float64(predictionWindowSize + 9)
	if samples[len(samples)-1].value != last {
		t.Errorf("마지막 값 = %f, 기대값 = %f", samples[len(samples)-1].value, last)
	}
}
