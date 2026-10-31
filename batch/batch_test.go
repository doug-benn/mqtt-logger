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

	bp := NewBatchProcessor(10, time.Hour, 2, func(items []int) {
		mu.Lock()
		got = append(got, items...)
		mu.Unlock()
	})

	for i := 0; i < 5; i++ {
		if err := bp.Add(context.Background(), i); err != nil {
			t.Fatalf("Add(%d): %v", i, err)
		}
	}
	bp.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 {
		t.Fatalf("expected 5 flushed items after Close, got %d", len(got))
	}
}

func TestFlushesWhenFull(t *testing.T) {
	flushed := make(chan []int, 1)
	bp := NewBatchProcessor(3, time.Hour, 2, func(items []int) {
		flushed <- items
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
	bp := NewBatchProcessor(10, 20*time.Millisecond, 2, func(items []int) {
		flushed <- items
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
	bp := NewBatchProcessor(10, time.Hour, 2, func([]int) {})
	bp.Close()
	if err := bp.Add(context.Background(), 1); err == nil {
		t.Fatal("expected error adding to a closed processor")
	}
	bp.Close()
}

// Add must not panic when it races with Close (the input channel is never
// closed; sends that lose the race are dropped or drained by run).
func TestAddConcurrentWithClose(t *testing.T) {
	bp := NewBatchProcessor(4, time.Hour, 2, func([]int) {})

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
	bp.Close()
	producers.Wait()
}

// Overflow must shed the newest arrivals with ErrBufferFull — bounded
// acceptance, no unbounded pile-up — while everything accepted still flushes
// once the database catches up.
func TestAddShedsNewestWhenQueueFull(t *testing.T) {
	var mu sync.Mutex
	var flushed []int
	release := make(chan struct{})
	bp := NewBatchProcessor(2, time.Hour, 1, func(items []int) {
		<-release // database "falls behind" until released
		mu.Lock()
		flushed = append(flushed, items...)
		mu.Unlock()
	})

	accepted := 0
	var shedErr error
	for i := 0; i < 500; i++ {
		err := bp.Add(context.Background(), i)
		if err == nil {
			accepted++
			continue
		}
		if !errors.Is(err, ErrBufferFull) {
			t.Fatalf("Add(%d) = %v, want ErrBufferFull", i, err)
		}
		shedErr = err
		break // shed-newest: stop at the first refusal
	}
	if shedErr == nil {
		t.Fatalf("Add never shed across 500 items (accepted %d) — unbounded acceptance, not bounded", accepted)
	}
	// Bounded means small: queue (4) + partial batch (2) + the batches held
	// up by the busy slot (2) — anything near 500 means pile-up returned.
	if accepted > 50 {
		t.Errorf("accepted %d items before shedding; want a bounded handful (queue + batches)", accepted)
	}

	close(release)
	bp.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != accepted {
		t.Errorf("flushed %d items, want all %d accepted ones — shedding must not lose what it accepted",
			len(flushed), accepted)
	}
}

// Concurrent flushes are capped at maxInFlight: run() must wait for a slot
// instead of spawning one goroutine per batch.
func TestInFlightFlushesBounded(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0

	bp := NewBatchProcessor(1, time.Hour, 2, func(items []int) {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
	})

	// The queue here is deliberately tiny (maxSize 1 → capacity 2), so a
	// burst may legitimately shed some items — the contract under test is
	// the in-flight cap, not acceptance.
	for i := 0; i < 20; i++ {
		if err := bp.Add(context.Background(), i); err != nil {
			if !errors.Is(err, ErrBufferFull) {
				t.Fatalf("Add(%d): %v", i, err)
			}
		}
	}
	bp.Close()

	mu.Lock()
	defer mu.Unlock()
	if peak > 2 {
		t.Errorf("peak concurrent flushes = %d, want ≤ 2 (the in-flight bound)", peak)
	}
	if peak < 1 {
		t.Error("no flush ever ran")
	}
}
