package batch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCloseFlushesPendingItems(t *testing.T) {
	var mu sync.Mutex
	var got []int

	bp := NewBatchProcessor(10, time.Hour, func(items []int) error {
		mu.Lock()
		got = append(got, items...)
		mu.Unlock()
		return nil
	})

	for i := 0; i < 5; i++ {
		if err := bp.Add(context.Background(), i); err != nil {
			t.Fatalf("Add(%d): %v", i, err)
		}
	}
	if err := bp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 {
		t.Fatalf("expected 5 flushed items after Close, got %d", len(got))
	}
}

func TestFlushesWhenFull(t *testing.T) {
	flushed := make(chan []int, 1)
	bp := NewBatchProcessor(3, time.Hour, func(items []int) error {
		flushed <- items
		return nil
	})
	defer bp.Close()

	for i := 0; i < 3; i++ {
		if err := bp.Add(context.Background(), i); err != nil {
			t.Fatalf("Add(%d): %v", i, err)
		}
	}

	select {
	case items := <-flushed:
		if len(items) != 3 {
			t.Fatalf("expected batch of 3, got %d", len(items))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("batch of 3 was never flushed")
	}
}

func TestFlushesOnTimeout(t *testing.T) {
	flushed := make(chan []int, 1)
	bp := NewBatchProcessor(10, 20*time.Millisecond, func(items []int) error {
		flushed <- items
		return nil
	})
	defer bp.Close()

	if err := bp.Add(context.Background(), 42); err != nil {
		t.Fatalf("Add: %v", err)
	}

	select {
	case items := <-flushed:
		if len(items) != 1 || items[0] != 42 {
			t.Fatalf("expected [42], got %v", items)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed-out batch was never flushed")
	}
}

func TestAddAfterCloseReturnsError(t *testing.T) {
	bp := NewBatchProcessor(10, time.Hour, func([]int) error { return nil })
	if err := bp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := bp.Add(context.Background(), 1); err == nil {
		t.Fatal("expected error adding to a closed processor")
	}
	if err := bp.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// Add must not panic when it races with Close (the input channel is never
// closed; sends that lose the race are dropped or drained by run).
func TestAddConcurrentWithClose(t *testing.T) {
	bp := NewBatchProcessor(4, time.Hour, func([]int) error { return nil })

	var producers sync.WaitGroup
	for g := 0; g < 8; g++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for i := 0; i < 200; i++ {
				_ = bp.Add(context.Background(), i)
			}
		}()
	}

	time.Sleep(5 * time.Millisecond)
	if err := bp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	producers.Wait()
}

func TestProcessorErrorDoesNotStopBatching(t *testing.T) {
	flushed := make(chan []int, 2)
	bp := NewBatchProcessor(1, time.Hour, func(items []int) error {
		flushed <- items
		return errors.New("boom")
	})
	defer bp.Close()

	if err := bp.Add(context.Background(), 1); err != nil {
		t.Fatalf("Add: %v", err)
	}
	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("first batch never flushed")
	}

	if err := bp.Add(context.Background(), 2); err != nil {
		t.Fatalf("Add after processor error: %v", err)
	}
	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("processor stopped after first error")
	}
}
