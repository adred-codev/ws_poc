package main

import (
	"context"
	"sync"
)

// Task represents a work item for the worker pool
type Task func()

// WorkerPool manages a fixed pool of worker goroutines
type WorkerPool struct {
	workerCount int
	taskQueue   chan Task
	ctx         context.Context
	wg          sync.WaitGroup
}

func NewWorkerPool(workerCount int) *WorkerPool {
	return &WorkerPool{
		workerCount: workerCount,
		taskQueue:   make(chan Task, workerCount*100), // Buffered queue
	}
}

func (wp *WorkerPool) Start(ctx context.Context) {
	wp.ctx = ctx
	
	// Start worker goroutines
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	
	for {
		select {
		case task := <-wp.taskQueue:
			if task != nil {
				task()
			}
		case <-wp.ctx.Done():
			return
		}
	}
}

func (wp *WorkerPool) Submit(task Task) {
	select {
	case wp.taskQueue <- task:
		// Task queued successfully
	default:
		// Queue is full, execute in current goroutine
		task()
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.taskQueue)
	wp.wg.Wait()
}
