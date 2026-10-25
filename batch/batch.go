package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type BatchProcessor[T any] struct {
	input     chan T
	maxSize   int
	timeout   time.Duration
	processor func([]T) error
	done      chan struct{}
	closed    int32
	wg        sync.WaitGroup
}

func NewBatchProcessor[T any](maxSize int, timeout time.Duration, processor func([]T) error) *BatchProcessor[T] {
	bp := &BatchProcessor[T]{
		input:     make(chan T, maxSize*2), // Buffer to prevent blocking
		maxSize:   maxSize,
		timeout:   timeout,
		processor: processor,
		done:      make(chan struct{}),
	}

	bp.wg.Add(1)
	go bp.run()
	return bp
}

func (bp *BatchProcessor[T]) Add(ctx context.Context, item T) error {
	if atomic.LoadInt32(&bp.closed) == 1 {
		return fmt.Errorf("batch processor is closed")
	}

	select {
	case bp.input <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-bp.done:
		return fmt.Errorf("batch processor is closed")
	}
}

func (bp *BatchProcessor[T]) run() {
	defer bp.wg.Done()

	var batch []T
	timer := time.NewTimer(bp.timeout)
	timer.Stop() // Stop initially

	flush := func() {
		if len(batch) > 0 {
			bp.processBatch(batch)
			batch = batch[:0]
		}
	}

	for {
		select {
		case item := <-bp.input:
			batch = append(batch, item)

			// Start timer if this is the first item
			if len(batch) == 1 {
				timer.Reset(bp.timeout)
			}

			// Check if batch is full
			if len(batch) >= bp.maxSize {
				timer.Stop()
				flush()
			}

		case <-timer.C:
			// Timeout occurred, flush current batch
			flush()

		case <-bp.done:
			// Drain items Add() accepted before shutdown so they flush too
			for {
				select {
				case item := <-bp.input:
					batch = append(batch, item)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (bp *BatchProcessor[T]) processBatch(batch []T) {
	// Make a copy to avoid data races
	batchCopy := make([]T, len(batch))
	copy(batchCopy, batch)

	// Add before starting: run() still holds a wg count here, so Close's Wait
	// can't observe zero and return while this batch is still in flight
	bp.wg.Add(1)
	go func() {
		defer bp.wg.Done()
		if err := bp.processor(batchCopy); err != nil {
			fmt.Printf("Error processing batch: %v\n", err)
		}
	}()
}

func (bp *BatchProcessor[T]) Close() error {
	if !atomic.CompareAndSwapInt32(&bp.closed, 0, 1) {
		return nil // Already closed
	}

	// input stays open: closing it races with in-flight Add sends and panics them
	close(bp.done)
	bp.wg.Wait()
	return nil
}
