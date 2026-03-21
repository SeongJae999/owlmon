package exporter

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// --- 직렬화 타입 ---

type fileRecord struct {
	Resource []serializedKV    `json:"r"`
	Scopes   []serializedScope `json:"s"`
}

type serializedKV struct {
	K string      `json:"k"`
	T string      `json:"t"` // bool, int64, float64, string
	V interface{} `json:"v"`
}

type serializedScope struct {
	Name    string             `json:"n"`
	Metrics []serializedMetric `json:"m"`
}

type serializedMetric struct {
	Name   string            `json:"name"`
	Unit   string            `json:"unit"`
	Type   string            `json:"type"` // gauge_f64, gauge_i64, sum_f64, sum_i64
	Mono   bool              `json:"mono,omitempty"`
	Temp   int               `json:"temp,omitempty"`
	Points []serializedPoint `json:"pts"`
}

type serializedPoint struct {
	Attrs []serializedKV `json:"a,omitempty"`
	ST    time.Time      `json:"st"`
	T     time.Time      `json:"t"`
	VF    float64        `json:"vf,omitempty"`
	VI    int64          `json:"vi,omitempty"`
}

// --- 직렬화 ---

func serializeRM(rm metricdata.ResourceMetrics) fileRecord {
	rec := fileRecord{}
	for _, kv := range rm.Resource.Attributes() {
		rec.Resource = append(rec.Resource, serializeKV(kv))
	}
	for _, sm := range rm.ScopeMetrics {
		scope := serializedScope{Name: sm.Scope.Name}
		for _, m := range sm.Metrics {
			scope.Metrics = append(scope.Metrics, serializeMetric(m))
		}
		rec.Scopes = append(rec.Scopes, scope)
	}
	return rec
}

func serializeMetric(m metricdata.Metrics) serializedMetric {
	sm := serializedMetric{Name: m.Name, Unit: m.Unit}
	switch data := m.Data.(type) {
	case metricdata.Gauge[float64]:
		sm.Type = "gauge_f64"
		for _, dp := range data.DataPoints {
			sm.Points = append(sm.Points, serializedPoint{
				Attrs: serializeAttrs(dp.Attributes),
				ST:    dp.StartTime,
				T:     dp.Time,
				VF:    dp.Value,
			})
		}
	case metricdata.Gauge[int64]:
		sm.Type = "gauge_i64"
		for _, dp := range data.DataPoints {
			sm.Points = append(sm.Points, serializedPoint{
				Attrs: serializeAttrs(dp.Attributes),
				ST:    dp.StartTime,
				T:     dp.Time,
				VI:    dp.Value,
			})
		}
	case metricdata.Sum[float64]:
		sm.Type = "sum_f64"
		sm.Mono = data.IsMonotonic
		sm.Temp = int(data.Temporality)
		for _, dp := range data.DataPoints {
			sm.Points = append(sm.Points, serializedPoint{
				Attrs: serializeAttrs(dp.Attributes),
				ST:    dp.StartTime,
				T:     dp.Time,
				VF:    dp.Value,
			})
		}
	case metricdata.Sum[int64]:
		sm.Type = "sum_i64"
		sm.Mono = data.IsMonotonic
		sm.Temp = int(data.Temporality)
		for _, dp := range data.DataPoints {
			sm.Points = append(sm.Points, serializedPoint{
				Attrs: serializeAttrs(dp.Attributes),
				ST:    dp.StartTime,
				T:     dp.Time,
				VI:    dp.Value,
			})
		}
	}
	return sm
}

func serializeAttrs(set attribute.Set) []serializedKV {
	kvs := set.ToSlice()
	result := make([]serializedKV, len(kvs))
	for i, kv := range kvs {
		result[i] = serializeKV(kv)
	}
	return result
}

func serializeKV(kv attribute.KeyValue) serializedKV {
	switch kv.Value.Type() {
	case attribute.BOOL:
		return serializedKV{K: string(kv.Key), T: "bool", V: kv.Value.AsBool()}
	case attribute.INT64:
		return serializedKV{K: string(kv.Key), T: "int64", V: kv.Value.AsInt64()}
	case attribute.FLOAT64:
		return serializedKV{K: string(kv.Key), T: "float64", V: kv.Value.AsFloat64()}
	default:
		return serializedKV{K: string(kv.Key), T: "string", V: kv.Value.AsString()}
	}
}

// --- 역직렬화 ---

func deserializeRM(rec fileRecord) metricdata.ResourceMetrics {
	attrs := make([]attribute.KeyValue, len(rec.Resource))
	for i, kv := range rec.Resource {
		attrs[i] = deserializeKV(kv)
	}
	res := resource.NewWithAttributes(semconv.SchemaURL, attrs...)

	scopeMetrics := make([]metricdata.ScopeMetrics, len(rec.Scopes))
	for i, scope := range rec.Scopes {
		sm := metricdata.ScopeMetrics{
			Scope: instrumentation.Scope{Name: scope.Name},
		}
		for _, m := range scope.Metrics {
			sm.Metrics = append(sm.Metrics, deserializeMetric(m))
		}
		scopeMetrics[i] = sm
	}
	return metricdata.ResourceMetrics{
		Resource:     res,
		ScopeMetrics: scopeMetrics,
	}
}

func deserializeMetric(m serializedMetric) metricdata.Metrics {
	metric := metricdata.Metrics{Name: m.Name, Unit: m.Unit}
	switch m.Type {
	case "gauge_f64":
		dps := make([]metricdata.DataPoint[float64], len(m.Points))
		for i, p := range m.Points {
			dps[i] = metricdata.DataPoint[float64]{
				Attributes: deserializeAttrs(p.Attrs),
				StartTime:  p.ST,
				Time:       p.T,
				Value:      p.VF,
			}
		}
		metric.Data = metricdata.Gauge[float64]{DataPoints: dps}
	case "gauge_i64":
		dps := make([]metricdata.DataPoint[int64], len(m.Points))
		for i, p := range m.Points {
			dps[i] = metricdata.DataPoint[int64]{
				Attributes: deserializeAttrs(p.Attrs),
				StartTime:  p.ST,
				Time:       p.T,
				Value:      p.VI,
			}
		}
		metric.Data = metricdata.Gauge[int64]{DataPoints: dps}
	case "sum_f64":
		dps := make([]metricdata.DataPoint[float64], len(m.Points))
		for i, p := range m.Points {
			dps[i] = metricdata.DataPoint[float64]{
				Attributes: deserializeAttrs(p.Attrs),
				StartTime:  p.ST,
				Time:       p.T,
				Value:      p.VF,
			}
		}
		metric.Data = metricdata.Sum[float64]{
			DataPoints:  dps,
			IsMonotonic: m.Mono,
			Temporality: metricdata.Temporality(m.Temp),
		}
	case "sum_i64":
		dps := make([]metricdata.DataPoint[int64], len(m.Points))
		for i, p := range m.Points {
			dps[i] = metricdata.DataPoint[int64]{
				Attributes: deserializeAttrs(p.Attrs),
				StartTime:  p.ST,
				Time:       p.T,
				Value:      p.VI,
			}
		}
		metric.Data = metricdata.Sum[int64]{
			DataPoints:  dps,
			IsMonotonic: m.Mono,
			Temporality: metricdata.Temporality(m.Temp),
		}
	}
	return metric
}

func deserializeAttrs(kvs []serializedKV) attribute.Set {
	attrs := make([]attribute.KeyValue, len(kvs))
	for i, kv := range kvs {
		attrs[i] = deserializeKV(kv)
	}
	return attribute.NewSet(attrs...)
}

func deserializeKV(s serializedKV) attribute.KeyValue {
	switch s.T {
	case "bool":
		v, _ := s.V.(bool)
		return attribute.Bool(s.K, v)
	case "int64":
		// JSON 숫자는 기본적으로 float64로 파싱됨
		switch v := s.V.(type) {
		case float64:
			return attribute.Int64(s.K, int64(v))
		case int64:
			return attribute.Int64(s.K, v)
		}
	case "float64":
		v, _ := s.V.(float64)
		return attribute.Float64(s.K, v)
	}
	v, _ := s.V.(string)
	return attribute.String(s.K, v)
}

// --- 파일 I/O ---

// saveBufferToFile은 버퍼 전체를 JSON 파일로 저장합니다.
func saveBufferToFile(path string, buffer []metricdata.ResourceMetrics) {
	if len(buffer) == 0 {
		// 버퍼 비었으면 파일 삭제
		_ = os.Remove(path)
		return
	}
	records := make([]fileRecord, len(buffer))
	for i, rm := range buffer {
		records[i] = serializeRM(rm)
	}
	data, err := json.Marshal(records)
	if err != nil {
		log.Printf("버퍼 파일 저장 실패 (직렬화): %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("버퍼 파일 저장 실패: %v", err)
	}
}

// loadBufferFromFile은 파일에서 버퍼를 복원합니다.
func loadBufferFromFile(path string) []metricdata.ResourceMetrics {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("버퍼 파일 읽기 실패: %v", err)
		}
		return nil
	}
	var records []fileRecord
	if err := json.Unmarshal(data, &records); err != nil {
		log.Printf("버퍼 파일 파싱 실패, 초기화합니다: %v", err)
		_ = os.Remove(path)
		return nil
	}
	result := make([]metricdata.ResourceMetrics, len(records))
	for i, rec := range records {
		result[i] = deserializeRM(rec)
	}
	log.Printf("이전 버퍼 복원 완료: %d개 배치 (재전송 예정)", len(result))
	return result
}
