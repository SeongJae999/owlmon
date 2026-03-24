package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/seongJae/owlmon/server/anomaly"
)

// AnomalyHandler는 이상탐지 결과를 API로 제공합니다.
type AnomalyHandler struct {
	detector  *anomaly.Detector
	predictor *anomaly.Predictor
}

func NewAnomalyHandler(detector *anomaly.Detector, predictor *anomaly.Predictor) *AnomalyHandler {
	return &AnomalyHandler{detector: detector, predictor: predictor}
}

// AnomalyResponse는 이상탐지 + 디스크 예측 통합 응답입니다.
type AnomalyResponse struct {
	Anomalies       []anomaly.Anomaly        `json:"anomalies"`
	DiskPredictions []anomaly.DiskPrediction  `json:"disk_predictions"`
	Stats           AnomalyStats              `json:"stats"`
}

type AnomalyStats struct {
	TrackedMetrics int `json:"tracked_metrics"`
	ActiveAnomalies int `json:"active_anomalies"`
}

// GetAnomalies는 현재 이상탐지 결과와 디스크 예측을 반환합니다.
// GET /api/anomaly
func (h *AnomalyHandler) GetAnomalies(w http.ResponseWriter, r *http.Request) {
	anomalies := h.detector.GetAnomalies()
	if anomalies == nil {
		anomalies = []anomaly.Anomaly{}
	}

	predictions := h.predictor.GetPredictions()
	if predictions == nil {
		predictions = []anomaly.DiskPrediction{}
	}

	windowCount, anomalyCount := h.detector.Stats()

	resp := AnomalyResponse{
		Anomalies:       anomalies,
		DiskPredictions: predictions,
		Stats: AnomalyStats{
			TrackedMetrics:  windowCount,
			ActiveAnomalies: anomalyCount,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetDiskPredictions는 디스크 고갈 예측만 반환합니다.
// GET /api/anomaly/disk
func (h *AnomalyHandler) GetDiskPredictions(w http.ResponseWriter, r *http.Request) {
	predictions := h.predictor.GetPredictions()
	if predictions == nil {
		predictions = []anomaly.DiskPrediction{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(predictions)
}

// InjectTestData는 테스트용 이상탐지 데이터를 주입합니다.
// POST /api/anomaly/test — 개발 환경에서 UI 확인용
func (h *AnomalyHandler) InjectTestData(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	h.detector.InjectTestAnomaly(anomaly.Anomaly{
		Host:       "web-server-01",
		Metric:     "cpu",
		Value:      94.2,
		ZScore:     4.12,
		Mean:       32.5,
		StdDev:     14.9,
		Severity:   "critical",
		Message:    "CPU 사용률이 평소 대비 비정상적으로 높음 (현재 94.2%, 평균 32.5%, Z=4.1)",
		DetectedAt: now,
	})
	h.detector.InjectTestAnomaly(anomaly.Anomaly{
		Host:       "db-server-01",
		Metric:     "memory",
		Value:      88.7,
		ZScore:     2.73,
		Mean:       55.2,
		StdDev:     12.3,
		Severity:   "warning",
		Message:    "메모리 사용률이 평소 대비 비정상적으로 높음 (현재 88.7%, 평균 55.2%, Z=2.7)",
		DetectedAt: now,
	})
	h.detector.InjectTestAnomaly(anomaly.Anomaly{
		Host:       "web-server-01",
		Metric:     "memory",
		Value:      91.3,
		ZScore:     3.55,
		Mean:       48.1,
		StdDev:     12.2,
		Severity:   "critical",
		Message:    "메모리 사용률이 평소 대비 비정상적으로 높음 (현재 91.3%, 평균 48.1%, Z=3.6)",
		DetectedAt: now,
	})

	h.predictor.InjectTestPrediction(anomaly.DiskPrediction{
		Host:       "file-server-01",
		Mountpoint: "/data",
		Current:    87.3,
		Slope:      0.42,
		DaysLeft:   4.2,
		R2:         0.91,
		Message:    "file-server-01 /data: 현재 87.3%, 약 4일 후 디스크 부족 예상",
	})
	h.predictor.InjectTestPrediction(anomaly.DiskPrediction{
		Host:       "web-server-01",
		Mountpoint: "/",
		Current:    72.1,
		Slope:      0.15,
		DaysLeft:   21.3,
		R2:         0.78,
		Message:    "web-server-01 /: 현재 72.1%, 약 21일 후 디스크 부족 예상",
	})
	h.predictor.InjectTestPrediction(anomaly.DiskPrediction{
		Host:       "db-server-01",
		Mountpoint: "/var/lib/postgresql",
		Current:    93.1,
		Slope:      1.85,
		DaysLeft:   0.4,
		R2:         0.95,
		Message:    "db-server-01 /var/lib/postgresql: 현재 93.1%, 24시간 내 디스크 부족 예상!",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "테스트 데이터 3건 이상 + 3건 디스크 예측 주입됨"})
}

// ClearTestData는 주입된 테스트 데이터를 초기화합니다.
// DELETE /api/anomaly/test
func (h *AnomalyHandler) ClearTestData(w http.ResponseWriter, r *http.Request) {
	h.detector.ClearAll()
	h.predictor.ClearAll()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "테스트 데이터 초기화됨"})
}
