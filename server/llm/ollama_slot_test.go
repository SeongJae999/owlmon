package llm

import (
	"context"
	"errors"
	"testing"
)

func TestCompleteStopsWaitingWhenContextEnds(t *testing.T) {
	ollamaSlot <- struct{}{}
	defer func() { <-ollamaSlot }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&OllamaProvider{URL: "http://127.0.0.1:1"}).Complete(ctx, "s", "u"); !errors.Is(err, context.Canceled) {
		t.Fatalf("대기 중 취소가 전달되지 않음: %v", err)
	}
}
