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

// BufferedExporter는 Collector 연결 실패 시 데이터를 메모리에 보관했다가
// 재연결되면 자동으로 재전송하는 Exporter 래퍼입니다.
type BufferedExporter struct {
	inner  metric.Exporter
	mu     sync.Mutex
	buffer []metricdata.ResourceMetrics
}

func NewBufferedExporter(inner metric.Exporter) *BufferedExporter {
	return &BufferedExporter{inner: inner}
}

func (b *BufferedExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	// 버퍼에 쌓인 데이터 먼저 전송 시도
	b.flushBuffer(ctx)

	if err := b.inner.Export(ctx, rm); err != nil {
		log.Printf("export 실패, 버퍼 저장 (현재 %d개): %v", b.bufferLen()+1, err)
		b.mu.Lock()
		defer b.mu.Unlock()
		if len(b.buffer) < maxBufferSize {
			b.buffer = append(b.buffer, *rm)
		} else {
			// 버퍼 꽉 차면 가장 오래된 데이터 제거
			b.buffer = append(b.buffer[1:], *rm)
			log.Printf("버퍼 초과, 가장 오래된 데이터 제거")
		}
		return err
	}
	return nil
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

	var remaining []metricdata.ResourceMetrics
	for _, rm := range pending {
		rmCopy := rm
		if err := b.inner.Export(ctx, &rmCopy); err != nil {
			remaining = append(remaining, rm)
			break // 전송 실패 시 이후 데이터도 유지
		}
	}

	b.mu.Lock()
	if len(remaining) == 0 {
		sent := len(pending)
		b.buffer = b.buffer[sent:]
		log.Printf("버퍼 플러시 완료: %d개 재전송", sent)
	} else {
		b.buffer = append(remaining, b.buffer[len(pending):]...)
	}
	b.mu.Unlock()
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
