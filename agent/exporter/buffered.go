package exporter

import (
	"context"
	"log"
	"sync"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const maxBufferSize = 100 // 최대 100개 배치 보관 (약 50분치)

// BufferedExporter는 Collector 연결 실패 시 데이터를 보관했다가
// 재연결되면 자동으로 재전송하는 Exporter 래퍼입니다.
// filePath가 지정되면 에이전트 재시작 후에도 버퍼가 유지됩니다.
type BufferedExporter struct {
	inner    metric.Exporter
	mu       sync.Mutex
	buffer   []metricdata.ResourceMetrics
	filePath string // "" = 파일 저장 비활성화
}

// NewBufferedExporter는 BufferedExporter를 생성합니다.
// filePath에 경로를 지정하면 버퍼를 파일로 영속화합니다.
// 시작 시 기존 파일이 있으면 자동으로 복원합니다.
func NewBufferedExporter(inner metric.Exporter, filePath string) *BufferedExporter {
	b := &BufferedExporter{inner: inner, filePath: filePath}
	if filePath != "" {
		b.buffer = loadBufferFromFile(filePath)
	}
	return b
}

func (b *BufferedExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	// 버퍼에 쌓인 데이터 먼저 전송 시도
	b.flushBuffer(ctx)

	if err := b.inner.Export(ctx, rm); err != nil {
		log.Printf("export 실패, 버퍼 저장 (현재 %d개): %v", b.bufferLen()+1, err)
		b.addToBuffer(*rm)
		return err
	}
	return nil
}

func (b *BufferedExporter) addToBuffer(rm metricdata.ResourceMetrics) {
	b.mu.Lock()
	if len(b.buffer) >= maxBufferSize {
		b.buffer = b.buffer[1:]
		log.Printf("버퍼 초과, 가장 오래된 데이터 제거")
	}
	b.buffer = append(b.buffer, rm)
	snapshot := b.snapshot()
	b.mu.Unlock()

	if b.filePath != "" {
		saveBufferToFile(b.filePath, snapshot)
	}
}

func (b *BufferedExporter) flushBuffer(ctx context.Context) {
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return
	}
	pending := make([]metricdata.ResourceMetrics, len(b.buffer))
	copy(pending, b.buffer)
	b.mu.Unlock()

	sentCount := 0
	for _, rm := range pending {
		rmCopy := rm
		if err := b.inner.Export(ctx, &rmCopy); err != nil {
			break // 실패 지점부터 이후 데이터 모두 유지
		}
		sentCount++
	}

	if sentCount == 0 {
		return
	}

	b.mu.Lock()
	b.buffer = b.buffer[sentCount:]
	snapshot := b.snapshot()
	b.mu.Unlock()

	if b.filePath != "" {
		saveBufferToFile(b.filePath, snapshot)
	}
	log.Printf("버퍼 플러시 완료: %d개 재전송, %d개 남음", sentCount, len(snapshot))
}

// snapshot은 mu를 보유한 상태에서 호출해야 합니다.
func (b *BufferedExporter) snapshot() []metricdata.ResourceMetrics {
	cp := make([]metricdata.ResourceMetrics, len(b.buffer))
	copy(cp, b.buffer)
	return cp
}

func (b *BufferedExporter) bufferLen() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buffer)
}

func (b *BufferedExporter) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	return b.inner.Temporality(kind)
}

func (b *BufferedExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return b.inner.Aggregation(kind)
}

func (b *BufferedExporter) ForceFlush(ctx context.Context) error {
	return b.inner.ForceFlush(ctx)
}

func (b *BufferedExporter) Shutdown(ctx context.Context) error {
	// 종료 전 버퍼 플러시 시도 (최대 10초)
	flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b.flushBuffer(flushCtx)
	return b.inner.Shutdown(ctx)
}
