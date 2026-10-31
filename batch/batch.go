package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrBufferFull is returned by Add when the input queue is at capacity: the
// reading is shed (drop-newest), never queued behind a backlog the database
// can't drain.
var ErrBufferFull = errors.New("batch buffer full")

type BatchProcessor[T any] struct {
	input   chan T
	maxSize int
	timeout time.Duration
	// processor runs each flush; it reports its own failures (the flush
	// closure owns logging, with the context it alone has) and so has no
	// error return to thread.
	processor func([]T)
	// slots bounds concurrent flushes; run() acquires one per batch, so a
	// full set pauses draining, backs up the input queue, and turns DB lag
	// into queue pressure instead of goroutine pile-up.
	slots  chan struct{}
	done   chan struct{}
	closed int32
	wg     sync.WaitGroup
}

// NewBatchProcessor buffers items up to maxSize (or timeout, whichever comes
// first) per flush. The input queue holds maxSize*2 items; while maxInFlight
// flushes are running, draining pauses, the queue backs up, and Add sheds the
// newest item with ErrBufferFull rather than blocking. Flushes run
// concurrently and unordered (up to maxInFlight), without the caller's
// context — the processor closure owns its own. maxInFlight is clamped to
// at least 1; a zero cap would stall draining on the first flush. Call
// Close once intake has drained: it flushes everything Add accepted.
func NewBatchProcessor[T any](maxSize int, timeout time.Duration, maxInFlight int, processor func([]T)) *BatchProcessor[T] {
	bp := &BatchProcessor[T]{
		input:     make(chan T, maxSize*2), // the queue whose fullness drives shedding
		maxSize:   maxSize,
		timeout:   timeout,
		processor: processor,
		slots:     make(chan struct{}, max(maxInFlight, 1)),
		done:      make(chan struct{}),
	}

	bp.wg.Add(1)
	go bp.run()
	return bp
}

// Add offers one item to the buffer and never waits: the item is accepted, or
// an error says why not — ErrBufferFull (queue at capacity), the processor is
// closed, or ctx is done. Under sustained overload the shed is the design,
// not a failure mode.
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
	default:
		// Queue at capacity: shed this newest item rather than block the
		// caller — memory stays flat when the database falls behind.
		return ErrBufferFull
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
	// Acquired in run()'s goroutine, so a full slot set pauses draining —
	// that pause is what fills the input queue and makes Add shed.
	bp.slots <- struct{}{}
	go func() {
		defer bp.wg.Done()
		defer func() { <-bp.slots }()
		bp.processor(batchCopy)
	}()
}

// Close flushes everything Add accepted, waiting for in-flight flushes to
// finish. It is idempotent and cannot fail: flush failures are the processor
// closure's to report. Shutdown lasts as long as the slowest flush — there is
// no deadline on the flush path.
func (bp *BatchProcessor[T]) Close() {
	if !atomic.CompareAndSwapInt32(&bp.closed, 0, 1) {
		return // Already closed
	}

	// input stays open: closing it races with in-flight Add sends and panics them
	close(bp.done)
	bp.wg.Wait()
}
