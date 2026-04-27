// Package ingest는 LogStore 앞단의 비동기 ingest 큐를 제공합니다.
//
// HTTP 핸들러가 받은 로그 배치를 채널 큐에 비동기로 위탁하고,
// 백그라운드 워커가 BatchSize 또는 FlushInterval 단위로
// PostgreSQL COPY 프로토콜 배치 삽입을 수행합니다.
//
// 이 분리의 이유:
//   - 핸들러는 200 OK를 즉시 반환 (DB 대기 없음)
//   - 트래픽 버스트를 채널 버퍼로 흡수
//   - DB-최적 배치 크기로 묶어서 처리량 극대화
package ingest

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/handler"
)

// Config는 Worker의 동작 파라미터입니다.
type Config struct {
	BufferSize    int           // 채널 버퍼 (총 보관 가능 레코드 수)
	BatchSize     int           // DB INSERT 배치 크기
	FlushInterval time.Duration // 배치가 안 차도 강제 flush하는 주기
	WorkerCount   int           // 워커 goroutine 수
}

// DefaultConfig는 Phase 0 권장 파라미터를 반환합니다.
func DefaultConfig() Config {
	return Config{
		BufferSize:    5000,
		BatchSize:     500,
		FlushInterval: 1 * time.Second,
		WorkerCount:   2,
	}
}

// Worker는 LogStore 앞단의 비동기 ingest 큐입니다.
//
// handler가 Enqueue로 레코드를 넣으면 워커 goroutine이
// 배치 크기(BatchSize) 또는 flush 주기(FlushInterval) 단위로
// LogStore.InsertBatch를 호출합니다.
type Worker struct {
	store *db.LogStore
	cfg   Config
	ch    chan db.LogRecord

	mu     sync.RWMutex
	closed bool

	wg sync.WaitGroup
}

// NewWorker는 워커를 생성합니다 (Start 호출 전엔 동작 안 함).
// cfg의 0/음수 필드는 DefaultConfig 값으로 대체됩니다.
func NewWorker(store *db.LogStore, cfg Config) *Worker {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 5000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 1 * time.Second
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 2
	}
	return &Worker{
		store: store,
		cfg:   cfg,
		ch:    make(chan db.LogRecord, cfg.BufferSize),
	}
}

// Start는 워커 goroutine들을 시작합니다.
// ctx가 취소되면 워커는 잔여 배치를 flush한 뒤 종료합니다.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.cfg.WorkerCount; i++ {
		w.wg.Add(1)
		go w.run(ctx, i)
	}
	log.Printf("[ingest] 워커 %d개 시작 (buffer=%d, batch=%d, flush=%v)",
		w.cfg.WorkerCount, w.cfg.BufferSize, w.cfg.BatchSize, w.cfg.FlushInterval)
}

// Enqueue는 레코드를 큐에 넣습니다.
// 큐가 가득 차면 handler.ErrIngestQueueFull을 반환합니다 (핸들러는 503으로 변환).
// Stop된 후에 호출하면 별도 에러를 반환합니다.
func (w *Worker) Enqueue(records []db.LogRecord) error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return errors.New("ingest worker is stopped")
	}
	for _, rec := range records {
		select {
		case w.ch <- rec:
			// ok
		default:
			return handler.ErrIngestQueueFull
		}
	}
	return nil
}

// Stop은 큐를 닫고 워커가 잔여 레코드를 flush할 시간을 줍니다.
// timeout 안에 모든 워커가 종료되지 않으면 손실 가능성이 있는 채로 반환합니다.
// 중복 호출은 안전합니다 (이미 closed면 no-op).
func (w *Worker) Stop(timeout time.Duration) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	close(w.ch)
	w.mu.Unlock()

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Println("[ingest] 워커 정상 종료")
	case <-time.After(timeout):
		log.Println("[ingest] drain timeout — 잔여 로그 손실 가능")
	}
}

// run은 단일 워커 goroutine입니다.
// ctx 취소 또는 채널 닫힘 시 잔여 배치를 flush하고 종료합니다.
func (w *Worker) run(ctx context.Context, id int) {
	defer w.wg.Done()

	batch := make([]db.LogRecord, 0, w.cfg.BatchSize)
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	// flush는 종료 시점에도 호출되므로 별도 ctx로 보호
	// (parent 취소돼도 30초까진 in-flight INSERT 시도).
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctxFlush, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := w.store.InsertBatch(ctxFlush, batch); err != nil {
			log.Printf("[ingest-%d] InsertBatch 실패: %v (손실 %d건)", id, err, len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case rec, ok := <-w.ch:
			if !ok {
				// 채널 닫힘 — 잔여 flush 후 종료
				flush()
				return
			}
			batch = append(batch, rec)
			if len(batch) >= w.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
