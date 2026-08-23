package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type task0030ContextKey struct{}

func TestTask0030StopPassesShutdownContextToFinalizationHook(t *testing.T) {
	var ticks atomic.Int32
	var finalized atomic.Int32
	runtime, err := NewRuntime("task0030", time.Millisecond, func(context.Context) error {
		ticks.Add(1)
		return nil
	}, func(ctx context.Context) error {
		if contextValue := ctx.Value(task0030ContextKey{}); contextValue != nil {
			finalized.Add(1)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for ticks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stopContext := context.WithValue(context.Background(), task0030ContextKey{}, "release-task0030")
	if err := runtime.Stop(stopContext); err != nil {
		t.Fatal(err)
	}
	if finalized.Load() != 1 {
		t.Fatalf("worker stop lost shutdown context; ticks=%d finalized=%d", ticks.Load(), finalized.Load())
	}
}
