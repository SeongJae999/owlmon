package anomaly

import (
	"math"
	"testing"
	"time"
)

// --- 시간대 분류 테스트 ---

func TestGetTimeSlot_업무시간(t *testing.T) {
	// 2026-03-24 화요일 10시
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.Local)
	if got := GetTimeSlot(ts); got != SlotBusinessHours {
		t.Errorf("화요일 10시 = %d, 기대값 = %d (업무시간)", got, SlotBusinessHours)
	}
}

func TestGetTimeSlot_야간(t *testing.T) {
	// 2026-03-24 화요일 22시
	ts := time.Date(2026, 3, 24, 22, 0, 0, 0, time.Local)
	if got := GetTimeSlot(ts); got != SlotNight {
		t.Errorf("화요일 22시 = %d, 기대값 = %d (야간)", got, SlotNight)
	}
}

func TestGetTimeSlot_주말(t *testing.T) {
	// 2026-03-22 일요일 14시
	ts := time.Date(2026, 3, 22, 14, 0, 0, 0, time.Local)
	if got := GetTimeSlot(ts); got != SlotWeekend {
		t.Errorf("일요일 14시 = %d, 기대값 = %d (주말)", got, SlotWeekend)
	}
}

func TestGetTimeSlot_업무시간_경계(t *testing.T) {
	// 09:00 → 업무시간
	ts9 := time.Date(2026, 3, 24, 9, 0, 0, 0, time.Local)
	if got := GetTimeSlot(ts9); got != SlotBusinessHours {
		t.Errorf("9시 = %d, 기대값 = 업무시간", got)
	}
	// 18:00 → 야간 (18시 이후)
	ts18 := time.Date(2026, 3, 24, 18, 0, 0, 0, time.Local)
	if got := GetTimeSlot(ts18); got != SlotNight {
		t.Errorf("18시 = %d, 기대값 = 야간", got)
	}
}

// --- 이동평균 윈도우 테스트 ---

func TestMetricWindow_평균_계산(t *testing.T) {
	w := newMetricWindow()
	w.Add(10)
	w.Add(20)
	w.Add(30)

	mean := w.Mean()
	if math.Abs(mean-20) > 0.01 {
		t.Errorf("평균 = %f, 기대값 = 20", mean)
	}
}

func TestMetricWindow_표준편차_계산(t *testing.T) {
	w := newMetricWindow()
	// 같은 값 → 표준편차 0
	for range 10 {
		w.Add(50)
	}
	if w.StdDev() > 0.01 {
		t.Errorf("같은 값의 표준편차 = %f, 기대값 ≈ 0", w.StdDev())
	}
}

func TestMetricWindow_순환버퍼_오버플로(t *testing.T) {
	w := newMetricWindow()
	// windowSize(120) + 10개 추가
	for i := range windowSize + 10 {
		w.Add(float64(i))
	}
	if w.count != windowSize {
		t.Errorf("count = %d, 기대값 = %d", w.count, windowSize)
	}
	// 최근 windowSize개의 평균: (10+11+...+129) / 120
	expectedMean := float64(10+129) / 2.0
	if math.Abs(w.Mean()-expectedMean) > 0.1 {
		t.Errorf("오버플로 후 평균 = %f, 기대값 ≈ %f", w.Mean(), expectedMean)
	}
}

func TestMetricWindow_Ready_최소샘플(t *testing.T) {
	w := newMetricWindow()
	for range minSamples - 1 {
		w.Add(50)
	}
	if w.Ready() {
		t.Error("minSamples-1개에서 Ready()가 true")
	}
	w.Add(50)
	if !w.Ready() {
		t.Error("minSamples개에서 Ready()가 false")
	}
}

// --- Detector 통합 테스트 ---

func TestDetector_정상범위_이상없음(t *testing.T) {
	d := NewDetector()
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.Local)

	// 안정적인 값 50%를 minSamples+10회 주입
	for range minSamples + 10 {
		a := d.Feed("server-01", "cpu", 50, ts)
		ts = ts.Add(30 * time.Second)
		if a != nil {
			t.Errorf("안정적인 값에서 이상 감지됨: %+v", a)
		}
	}

	anomalies := d.GetAnomalies()
	if len(anomalies) != 0 {
		t.Errorf("이상 %d건, 기대값 = 0", len(anomalies))
	}
}

func TestDetector_급격한_변화_이상감지(t *testing.T) {
	d := NewDetector()
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.Local)

	// 안정적인 값 30% 주입 (약간의 변동 포함)
	for i := range minSamples + 20 {
		val := 30.0 + float64(i%5)*0.5 // 30~32 범위
		d.Feed("server-01", "cpu", val, ts)
		ts = ts.Add(30 * time.Second)
	}

	// 갑자기 95%로 급등
	a := d.Feed("server-01", "cpu", 95, ts)
	if a == nil {
		t.Fatal("급격한 CPU 급등에서 이상을 감지하지 못함")
	}
	if a.Severity != "critical" {
		t.Errorf("severity = %s, 기대값 = critical", a.Severity)
	}
	if a.ZScore < zThreshold {
		t.Errorf("Z-score = %f, 기대값 ≥ %f", a.ZScore, zThreshold)
	}
}

func TestDetector_복구시_이상해제(t *testing.T) {
	d := NewDetector()
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.Local)

	// 안정 → 이상 → 복구
	for i := range minSamples + 20 {
		val := 30.0 + float64(i%3)*0.3
		d.Feed("server-01", "cpu", val, ts)
		ts = ts.Add(30 * time.Second)
	}

	// 이상 발생
	d.Feed("server-01", "cpu", 95, ts)
	ts = ts.Add(30 * time.Second)

	if len(d.GetHostAnomalies("server-01")) == 0 {
		t.Fatal("이상이 기록되지 않음")
	}

	// 정상 복구
	d.Feed("server-01", "cpu", 30, ts)

	if len(d.GetHostAnomalies("server-01")) != 0 {
		t.Error("정상 복구 후에도 이상이 남아있음")
	}
}

func TestDetector_계절성_시간대별_별도_통계(t *testing.T) {
	d := NewDetector()

	// 업무시간에 데이터 축적
	tsBusiness := time.Date(2026, 3, 24, 10, 0, 0, 0, time.Local) // 화요일 10시
	for range minSamples + 10 {
		d.Feed("server-01", "cpu", 70, tsBusiness)
		tsBusiness = tsBusiness.Add(30 * time.Second)
	}

	// 야간에 데이터 축적 (낮은 값)
	tsNight := time.Date(2026, 3, 24, 23, 0, 0, 0, time.Local) // 화요일 23시
	for range minSamples + 10 {
		d.Feed("server-01", "cpu", 10, tsNight)
		tsNight = tsNight.Add(30 * time.Second)
	}

	// 야간 기준으로 70%는 이상
	a := d.Feed("server-01", "cpu", 70, tsNight)
	if a == nil {
		t.Fatal("야간에 업무시간 수준의 CPU가 이상으로 감지되지 않음")
	}

	// 업무시간 기준으로 70%는 정상
	a = d.Feed("server-01", "cpu", 70, tsBusiness)
	if a != nil {
		t.Errorf("업무시간에 평소 수준의 CPU가 이상으로 감지됨: Z=%f", a.ZScore)
	}
}

func TestDetector_Stats(t *testing.T) {
	d := NewDetector()
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.Local)

	for range 10 {
		d.Feed("server-01", "cpu", 50, ts)
		d.Feed("server-01", "memory", 60, ts)
		ts = ts.Add(30 * time.Second)
	}

	wc, ac := d.Stats()
	if wc != 2 {
		t.Errorf("추적 메트릭 수 = %d, 기대값 = 2", wc)
	}
	if ac != 0 {
		t.Errorf("활성 이상 수 = %d, 기대값 = 0", ac)
	}
}
