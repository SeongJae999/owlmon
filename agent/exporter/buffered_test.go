package exporter

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// --- 테스트용 Exporter ---

// fakeExporter는 failNext 횟수만큼 실패 후 이후 모두 성공합니다.
type fakeExporter struct {
	exported []metricdata.ResourceMetrics
	failNext int
}

func (f *fakeExporter) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	if f.failNext > 0 {
		f.failNext--
		return errors.New("전송 실패 (테스트)")
	}
	f.exported = append(f.exported, *rm)
	return nil
}
func (f *fakeExporter) Temporality(metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}
func (f *fakeExporter) Aggregation(k metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(k)
}
func (f *fakeExporter) ForceFlush(context.Context) error { return nil }
func (f *fakeExporter) Shutdown(context.Context) error   { return nil }

// partialSuccessExporter는 successCount번 성공 후 항상 실패합니다.
type partialSuccessExporter struct {
	exported     []metricdata.ResourceMetrics
	successCount int
}

func (p *partialSuccessExporter) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	if p.successCount > 0 {
		p.successCount--
		p.exported = append(p.exported, *rm)
		return nil
	}
	return errors.New("전송 실패 (테스트)")
}
func (p *partialSuccessExporter) Temporality(metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}
func (p *partialSuccessExporter) Aggregation(k metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(k)
}
func (p *partialSuccessExporter) ForceFlush(context.Context) error { return nil }
func (p *partialSuccessExporter) Shutdown(context.Context) error   { return nil }

// --- 테스트용 헬퍼 ---

func makeRM(name string, value float64) metricdata.ResourceMetrics {
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.HostName("test-host"),
	)
	return metricdata.ResourceMetrics{
		Resource: res,
		ScopeMetrics: []metricdata.ScopeMetrics{{
			Metrics: []metricdata.Metrics{{
				Name: name,
				Data: metricdata.Gauge[float64]{
					DataPoints: []metricdata.DataPoint[float64]{{
						Time:  time.Now(),
						Value: value,
					}},
				},
			}},
		}},
	}
}

// setBuffer는 같은 패키지 테스트에서 버퍼를 직접 설정합니다.
func setBuffer(b *BufferedExporter, items []metricdata.ResourceMetrics) {
	b.mu.Lock()
	b.buffer = items
	b.mu.Unlock()
}

// --- 버퍼링 로직 테스트 ---

func TestExport_실패시_버퍼에_저장됨(t *testing.T) {
	fake := &fakeExporter{failNext: 1}
	b := NewBufferedExporter(fake, "")

	rm := makeRM("cpu", 42.0)
	_ = b.Export(context.Background(), &rm)

	if b.bufferLen() != 1 {
		t.Errorf("버퍼 크기 = %d, 기대값 = 1", b.bufferLen())
	}
}

func TestExport_성공시_버퍼_비어있음(t *testing.T) {
	fake := &fakeExporter{}
	b := NewBufferedExporter(fake, "")

	rm := makeRM("cpu", 42.0)
	if err := b.Export(context.Background(), &rm); err != nil {
		t.Fatalf("export 실패: %v", err)
	}

	if b.bufferLen() != 0 {
		t.Errorf("버퍼 크기 = %d, 기대값 = 0", b.bufferLen())
	}
}

func TestFlushBuffer_재연결시_버퍼_순서대로_재전송(t *testing.T) {
	fake := &fakeExporter{}
	b := NewBufferedExporter(fake, "")

	// 버퍼에 3개 직접 설정 (같은 패키지라 가능)
	setBuffer(b, []metricdata.ResourceMetrics{
		makeRM("cpu", 0.0),
		makeRM("cpu", 1.0),
		makeRM("cpu", 2.0),
	})

	// 새 Export 호출 → flushBuffer 발동 → 버퍼 3개 재전송 + 새 1개
	rm := makeRM("cpu", 99.0)
	if err := b.Export(context.Background(), &rm); err != nil {
		t.Fatalf("export 실패: %v", err)
	}

	if b.bufferLen() != 0 {
		t.Errorf("버퍼 크기 = %d, 기대값 = 0", b.bufferLen())
	}
	if len(fake.exported) != 4 {
		t.Errorf("전송된 개수 = %d, 기대값 = 4", len(fake.exported))
	}
	// 순서 확인: 버퍼 순서(0,1,2) 먼저, 새 데이터(99) 마지막
	lastVal := fake.exported[3].ScopeMetrics[0].Metrics[0].Data.(metricdata.Gauge[float64]).DataPoints[0].Value
	if lastVal != 99.0 {
		t.Errorf("마지막 전송값 = %f, 기대값 = 99.0", lastVal)
	}
}

func TestFlushBuffer_중간_실패시_이후_데이터_보존(t *testing.T) {
	// 이 테스트가 방금 수정한 버그를 커버함
	// 버퍼 [0,1,2,3,4] 중 2개 전송 성공 → 나머지 [2,3,4] 보존돼야 함
	b := NewBufferedExporter(&fakeExporter{}, "")
	setBuffer(b, []metricdata.ResourceMetrics{
		makeRM("cpu", 0.0),
		makeRM("cpu", 1.0),
		makeRM("cpu", 2.0),
		makeRM("cpu", 3.0),
		makeRM("cpu", 4.0),
	})

	// 2개만 성공하는 exporter로 교체
	partial := &partialSuccessExporter{successCount: 2}
	b.inner = partial
	b.flushBuffer(context.Background())

	if b.bufferLen() != 3 {
		t.Errorf("버퍼 크기 = %d, 기대값 = 3 (2개 전송 후 3개 남아야 함)", b.bufferLen())
	}
	if len(partial.exported) != 2 {
		t.Errorf("전송된 개수 = %d, 기대값 = 2", len(partial.exported))
	}

	// 남은 첫 번째 값이 2.0인지 확인 (0,1이 나가고 2부터 남아야 함)
	b.mu.Lock()
	firstRemaining := b.buffer[0].ScopeMetrics[0].Metrics[0].Data.(metricdata.Gauge[float64]).DataPoints[0].Value
	b.mu.Unlock()
	if firstRemaining != 2.0 {
		t.Errorf("남은 첫 번째 값 = %f, 기대값 = 2.0", firstRemaining)
	}
}

func TestBuffer_maxBufferSize_초과시_오래된것_제거(t *testing.T) {
	// addToBuffer 직접 호출: flushBuffer 개입 없이 순수하게 버퍼만 채움
	b := NewBufferedExporter(&fakeExporter{}, "")

	for i := range maxBufferSize + 1 {
		b.addToBuffer(makeRM("cpu", float64(i)))
	}

	if b.bufferLen() != maxBufferSize {
		t.Errorf("버퍼 크기 = %d, 기대값 = %d", b.bufferLen(), maxBufferSize)
	}

	// 가장 오래된(i=0) 데이터가 제거되고 i=1부터 시작해야 함
	b.mu.Lock()
	firstVal := b.buffer[0].ScopeMetrics[0].Metrics[0].Data.(metricdata.Gauge[float64]).DataPoints[0].Value
	b.mu.Unlock()
	if firstVal != 1.0 {
		t.Errorf("첫 번째 값 = %f, 기대값 = 1.0 (i=0이 제거됐어야 함)", firstVal)
	}
}

// --- 파일 영속성 테스트 ---

func TestPersistence_저장_후_재로드(t *testing.T) {
	tmpFile := t.TempDir() + "/test-buffer.json"

	b := NewBufferedExporter(&fakeExporter{}, tmpFile)

	// addToBuffer로 직접 추가 (파일 저장 트리거)
	b.addToBuffer(makeRM("cpu", 55.5))
	b.addToBuffer(makeRM("memory", 80.0))

	if b.bufferLen() != 2 {
		t.Fatalf("버퍼 크기 = %d, 기대값 = 2", b.bufferLen())
	}
	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatalf("버퍼 파일이 생성되지 않음: %v", err)
	}

	// 새 인스턴스로 파일 복원
	fake2 := &fakeExporter{}
	b2 := NewBufferedExporter(fake2, tmpFile)

	if b2.bufferLen() != 2 {
		t.Errorf("복원된 버퍼 크기 = %d, 기대값 = 2", b2.bufferLen())
	}

	// 복원 후 새 export → 버퍼 2개 재전송 + 새 1개 = 총 3개
	rm := makeRM("disk", 90.0)
	if err := b2.Export(context.Background(), &rm); err != nil {
		t.Fatalf("복원 후 export 실패: %v", err)
	}
	if len(fake2.exported) != 3 {
		t.Errorf("전송된 개수 = %d, 기대값 = 3", len(fake2.exported))
	}
}

func TestPersistence_버퍼_비면_파일_삭제(t *testing.T) {
	tmpFile := t.TempDir() + "/test-buffer.json"

	fake := &fakeExporter{failNext: 1}
	b := NewBufferedExporter(fake, tmpFile)

	// 실패 → 파일 생성
	rm := makeRM("cpu", 42.0)
	_ = b.Export(context.Background(), &rm)

	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatalf("버퍼 파일이 없음: %v", err)
	}

	// 성공 → flush → 파일 삭제
	rm2 := makeRM("cpu", 43.0)
	_ = b.Export(context.Background(), &rm2)

	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("버퍼가 비었는데 파일이 남아있음")
	}
}

func TestPersistence_파일_없을때_로드(t *testing.T) {
	result := loadBufferFromFile("/nonexistent/path/buffer.json")
	if result != nil {
		t.Errorf("결과 = %v, 기대값 = nil", result)
	}
}

func TestPersistence_손상된_파일_처리(t *testing.T) {
	tmpFile := t.TempDir() + "/corrupt-buffer.json"
	_ = os.WriteFile(tmpFile, []byte("{ 잘못된 JSON !!!"), 0600)

	result := loadBufferFromFile(tmpFile)
	if result != nil {
		t.Errorf("결과 = %v, 기대값 = nil", result)
	}
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("손상된 파일이 삭제되지 않음")
	}
}

// --- 직렬화 왕복 테스트 ---

func TestSerialization_Gauge_float64_왕복(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := metricdata.ResourceMetrics{
		Resource: resource.NewWithAttributes(semconv.SchemaURL,
			semconv.HostName("server-01"),
		),
		ScopeMetrics: []metricdata.ScopeMetrics{{
			Metrics: []metricdata.Metrics{{
				Name: "system.cpu.usage",
				Unit: "%",
				Data: metricdata.Gauge[float64]{
					DataPoints: []metricdata.DataPoint[float64]{{
						Attributes: attribute.NewSet(attribute.String("state", "user")),
						Time:       now,
						Value:      73.5,
					}},
				},
			}},
		}},
	}

	restored := deserializeRM(serializeRM(original))

	// 메트릭 이름
	if original.ScopeMetrics[0].Metrics[0].Name != restored.ScopeMetrics[0].Metrics[0].Name {
		t.Errorf("메트릭 이름 불일치")
	}

	origDP := original.ScopeMetrics[0].Metrics[0].Data.(metricdata.Gauge[float64]).DataPoints[0]
	restDP := restored.ScopeMetrics[0].Metrics[0].Data.(metricdata.Gauge[float64]).DataPoints[0]

	if origDP.Value != restDP.Value {
		t.Errorf("값 = %f, 기대값 = %f", restDP.Value, origDP.Value)
	}
	if !origDP.Time.Equal(restDP.Time) {
		t.Errorf("시간 불일치: %v != %v", restDP.Time, origDP.Time)
	}
}

func TestSerialization_Sum_int64_왕복(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := metricdata.ResourceMetrics{
		Resource: resource.NewWithAttributes(semconv.SchemaURL,
			semconv.HostName("server-01"),
		),
		ScopeMetrics: []metricdata.ScopeMetrics{{
			Metrics: []metricdata.Metrics{{
				Name: "system.network.io",
				Unit: "By",
				Data: metricdata.Sum[int64]{
					IsMonotonic: true,
					Temporality: metricdata.CumulativeTemporality,
					DataPoints: []metricdata.DataPoint[int64]{{
						Attributes: attribute.NewSet(
							attribute.String("direction", "receive"),
							attribute.String("device", "eth0"),
						),
						Time:  now,
						Value: 1234567890,
					}},
				},
			}},
		}},
	}

	restored := deserializeRM(serializeRM(original))

	origData := original.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64])
	restData := restored.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64])

	if origData.IsMonotonic != restData.IsMonotonic {
		t.Errorf("IsMonotonic = %v, 기대값 = %v", restData.IsMonotonic, origData.IsMonotonic)
	}
	// int64 값 정확도 확인 (JSON 변환 시 float64 경유로 인한 손실 여부)
	if origData.DataPoints[0].Value != restData.DataPoints[0].Value {
		t.Errorf("int64 값 = %d, 기대값 = %d", restData.DataPoints[0].Value, origData.DataPoints[0].Value)
	}
	if len(origData.DataPoints[0].Attributes.ToSlice()) != len(restData.DataPoints[0].Attributes.ToSlice()) {
		t.Errorf("속성 개수 불일치")
	}
}

func TestSerialization_리소스_호스트명_보존(t *testing.T) {
	original := metricdata.ResourceMetrics{
		Resource: resource.NewWithAttributes(semconv.SchemaURL,
			semconv.HostName("my-server"),
			attribute.String("env", "production"),
		),
		ScopeMetrics: []metricdata.ScopeMetrics{},
	}

	restored := deserializeRM(serializeRM(original))

	origAttrs := original.Resource.Attributes()
	restAttrs := restored.Resource.Attributes()
	if len(origAttrs) != len(restAttrs) {
		t.Errorf("리소스 속성 개수 = %d, 기대값 = %d", len(restAttrs), len(origAttrs))
	}
}
