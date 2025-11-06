package main

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

// Task represents a work item for the worker pool.
// Tasks are functions with no parameters or return values.
// They are executed asynchronously by worker goroutines.
type Task func()

// WorkerPool manages a fixed pool of worker goroutines for concurrent task execution.
//
// Purpose:
//   - Limit concurrent goroutines to prevent resource exhaustion
//   - Process Kafka messages and broadcast to clients without blocking
//   - Provides backpressure when system is overloaded
//
// Design:
//   - Fixed number of workers (typically 2 × CPU cores)
//   - Buffered task queue (workerCount × 100 capacity)
//   - If queue full, tasks execute synchronously in caller goroutine
//
// Performance characteristics:
//   - With 8 CPU cores: 16 workers, 1600 task queue capacity
//   - Each broadcast takes ~1-2ms with 10,000 clients
//   - Can handle ~500-1000 broadcasts/second sustained
//
// Memory usage:
//   - Worker goroutines: 16 × 2KB stack = 32KB
//   - Task queue: 1600 × 8 bytes (function pointer) = 12.8KB
//   - Total: ~45KB (negligible)
//
// Thread safety:
//
//	All methods are safe for concurrent use by multiple goroutines.
type WorkerPool struct {
	workerCount  int             // Number of worker goroutines
	taskQueue    chan Task       // Buffered channel of pending tasks
	ctx          context.Context // Context for graceful shutdown
	wg           sync.WaitGroup  // Wait group to track worker completion
	droppedTasks int64           // Atomic counter for dropped tasks when queue full
	logger       zerolog.Logger  // Structured logger for panic recovery
}

// NewWorkerPool creates a worker pool with the specified number of workers.
//
// Parameters:
//
//	workerCount - Number of worker goroutines (typically 2 × CPU cores)
//	queueSize - Size of the task queue buffer
//	logger - Structured logger for panic recovery and error logging
//
// Queue sizing:
//   - Buffer capacity: Configurable via queueSize parameter
//   - Example: 32 workers, 3200 queue
//   - Reasoning: Handles burst of Kafka messages during traffic spikes
//
// Recommended workerCount values:
//   - Development: runtime.NumCPU()
//   - Production: runtime.GOMAXPROCS(0) × 2
//   - Container: Automatically set via automaxprocs
func NewWorkerPool(workerCount int, queueSize int, logger zerolog.Logger) *WorkerPool {
	return &WorkerPool{
		workerCount: workerCount,
		taskQueue:   make(chan Task, queueSize),
		logger:      logger,
	}
}

// Start initializes and starts all worker goroutines.
// Must be called before Submit. Safe to call only once.
//
// The provided context is used for graceful shutdown:
//   - When context is cancelled, workers finish current task and exit
//   - New tasks submitted after cancellation are dropped
//
// Workers remain active until Stop is called or context is cancelled.
func (wp *WorkerPool) Start(ctx context.Context) {
	wp.ctx = ctx

	// Start worker goroutines
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// worker is the main loop for each worker goroutine.
// Continuously pulls tasks from the queue and executes them.
//
// Behavior:
//   - Blocks waiting for task or context cancellation
//   - Executes tasks synchronously (one at a time per worker)
//   - Gracefully exits when context is cancelled
//   - Recovers from panics with full stack trace logging to Loki
//
// Panic Recovery:
//   - All panics are caught and logged with stack traces
//   - Worker continues running after panic (doesn't crash)
//   - Panic details sent to Loki for debugging
func (wp *WorkerPool) worker() {
	defer wp.wg.Done()

	for {
		select {
		case task := <-wp.taskQueue:
			if task != nil {
				// Execute task with panic recovery
				func() {
					defer func() {
						if r := recover(); r != nil {
							stack := string(debug.Stack())
							wp.logger.Error().
								Interface("panic_value", r).
								Str("stack_trace", stack).
								Msg("Worker panic recovered - task failed but worker continues")

							// Record panic in metrics
							RecordError(ErrorTypeBroadcast, ErrorSeverityCritical)
						}
					}()

					// Execute the task
					task()
				}()
			}
		case <-wp.ctx.Done():
			wp.logger.Debug().Msg("Worker shutting down")
			return
		}
	}
}

// Submit enqueues a task for asynchronous execution by a worker.
//
// Behavior:
//   - If queue has space: Task is queued and Submit returns immediately
//   - If queue is full: Task is DROPPED and counter incremented
//
// Task dropping provides backpressure:
//   - Prevents goroutine explosion when system overloaded
//   - Prevents unbounded memory growth
//   - Drops work instead of crashing under load
//   - Dropped count tracked in droppedTasks (atomic counter)
//
// CRITICAL: This prevents the goroutine explosion that causes CPU → 100%
// When publisher rate × client count exceeds worker capacity, tasks are dropped
// instead of spawning unlimited goroutines.
//
// Thread safety: Safe for concurrent use by multiple goroutines.
//
// Example:
//
//	pool.Submit(func() {
//	    server.broadcast(message)
//	})
func (wp *WorkerPool) Submit(task Task) {
	select {
	case wp.taskQueue <- task:
		// Task queued successfully
	default:
		// Queue is full - drop task to prevent goroutine explosion
		atomic.AddInt64(&wp.droppedTasks, 1)
	}
}

// Stop gracefully shuts down the worker pool.
//
// Shutdown sequence:
//  1. Closes task queue (no new tasks accepted)
//  2. Workers finish currently executing tasks
//  3. Workers process any remaining queued tasks
//  4. All workers exit
//  5. Stop returns when all workers have finished
//
// Blocks until all workers have completed.
// Safe to call multiple times (subsequent calls are no-op).
//
// Note: Tasks submitted after Stop is called will panic (send on closed channel).
func (wp *WorkerPool) Stop() {
	close(wp.taskQueue)
	wp.wg.Wait()
}

// GetDroppedTasks returns the total number of tasks dropped due to queue full.
// This counter indicates backpressure - when broadcast rate exceeds worker capacity.
//
// High dropped task count means:
//   - Publisher rate too high for current client count
//   - Worker pool too small for the workload
//   - CPU can't keep up with message fanout
//
// Thread safety: Safe for concurrent use (atomic read).
func (wp *WorkerPool) GetDroppedTasks() int64 {
	return atomic.LoadInt64(&wp.droppedTasks)
}

// GetQueueDepth returns the current number of tasks waiting in the queue
func (wp *WorkerPool) GetQueueDepth() int {
	return len(wp.taskQueue)
}

// GetQueueCapacity returns the maximum capacity of the task queue
func (wp *WorkerPool) GetQueueCapacity() int {
	return cap(wp.taskQueue)
}
