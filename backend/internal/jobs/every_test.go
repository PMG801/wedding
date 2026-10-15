package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEveryRunsImmediatelyThenOnIntervalUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		Every(ctx, 10*time.Millisecond, func(context.Context) { runs.Add(1) })
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for runs.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("job ran %d times; want at least 3 (startup plus intervals)", runs.Load())
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Every did not return after cancellation")
	}
}

func TestEveryRunsAtStartupBeforeFirstInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan struct{}, 1)
	go Every(ctx, time.Hour, func(context.Context) { ran <- struct{}{} })
	defer cancel()
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("job did not run at startup")
	}
}
