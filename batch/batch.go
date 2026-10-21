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
		fmt.Println("batch")
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

	for {
		select {
		case item, ok := <-bp.input:
			if !ok {
				// Channel closed, flush and exit
				if len(batch) > 0 {
					bp.processBatch(batch)
				}
				return
			}

			batch = append(batch, item)

			// Start timer if this is the first item
			if len(batch) == 1 {
				timer.Reset(bp.timeout)
			}

			// Check if batch is full
			if len(batch) >= bp.maxSize {
				timer.Stop()
				bp.processBatch(batch)
				batch = batch[:0] // Reset slice
			}

		case <-timer.C:
			// Timeout occurred, flush current batch
			if len(batch) > 0 {
				bp.processBatch(batch)
				batch = batch[:0] // Reset slice
			}

		case <-bp.done:
			// Flush remaining items and exit
			if len(batch) > 0 {
				bp.processBatch(batch)
			}
			return
		}
	}
}

func (bp *BatchProcessor[T]) processBatch(batch []T) {
	// Make a copy to avoid data races
	batchCopy := make([]T, len(batch))
	copy(batchCopy, batch)

	go func() {
		if err := bp.processor(batchCopy); err != nil {
			fmt.Printf("Error processing batch: %v\n", err)
		}
	}()
}

func (bp *BatchProcessor[T]) Close() error {
	if !atomic.CompareAndSwapInt32(&bp.closed, 0, 1) {
		return nil // Already closed
	}

	close(bp.done)
	close(bp.input)
	bp.wg.Wait()
	return nil
}
