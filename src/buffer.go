package main

import (
	"sync"
)

// BufferPool manages a pool of reusable byte buffers for memory efficiency.
//
// Purpose:
//   - Reduce garbage collection pressure by reusing buffers
//   - Avoid repeated allocations for temporary I/O operations
//   - Tiered sizing (small/medium/large) for efficient memory usage
//
// Buffer sizes:
//   - Small:  4KB   (typical WebSocket message)
//   - Medium: 16KB  (large messages, batched data)
//   - Large:  64KB  (maximum WebSocket frame size)
//
// Performance benefits:
//   - Eliminates allocation overhead for hot paths
//   - Reduces GC pressure (fewer short-lived objects)
//   - Memory reuse reduces overall heap size
//
// Memory characteristics:
//   - Buffers are cleared (length set to 0) before returning to pool
//   - Capacity is preserved (underlying array not reallocated)
//   - sync.Pool may evict unused buffers during GC
//
// Thread safety:
//   All methods are safe for concurrent use by multiple goroutines.
type BufferPool struct {
	small  sync.Pool // 4KB buffers
	medium sync.Pool // 16KB buffers
	large  sync.Pool // 64KB buffers
}

// NewBufferPool creates a buffer pool with three size tiers.
//
// Parameters:
//   defaultSize - Currently unused, kept for API compatibility
//
// Buffer tiers created:
//   - Small:  4KB  (4096 bytes)
//   - Medium: 16KB (16384 bytes)
//   - Large:  64KB (65536 bytes)
//
// Implementation note:
//   Uses sync.Pool which automatically manages buffer lifecycle.
//   The Go runtime may garbage collect unused buffers during GC cycles.
func NewBufferPool(defaultSize int) *BufferPool {
	return &BufferPool{
		small: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 4096)
				return &buf
			},
		},
		medium: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 16384)
				return &buf
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 65536)
				return &buf
			},
		},
	}
}

// Get retrieves a buffer from the pool suitable for the requested size.
//
// Buffer selection:
//   - size ≤ 4KB:  Returns 4KB buffer from small pool
//   - size ≤ 16KB: Returns 16KB buffer from medium pool
//   - size > 16KB: Returns 64KB buffer from large pool
//
// Behavior:
//   - If pool has available buffer: Returns pooled buffer
//   - If pool is empty: Allocates new buffer of appropriate size
//
// The returned buffer may be larger than requested (for efficiency).
// Caller should use returned buffer's capacity, not the requested size.
//
// Thread safety: Safe for concurrent use by multiple goroutines.
//
// Example:
//   buf := pool.Get(2048)    // Gets 4KB buffer
//   defer pool.Put(buf)       // Return to pool when done
//   // Use buf for I/O operations
func (bp *BufferPool) Get(size int) *[]byte {
	var pool *sync.Pool

	switch {
	case size <= 4096:
		pool = &bp.small
	case size <= 16384:
		pool = &bp.medium
	default:
		pool = &bp.large
	}

	v := pool.Get()
	if buf, ok := v.(*[]byte); ok {
		return buf
	}

	// Fallback: create new buffer
	buf := make([]byte, size)
	return &buf
}

// Put returns a buffer to the pool for reuse.
//
// Buffer routing (by capacity):
//   - capacity ≤ 4KB:  Returned to small pool
//   - capacity ≤ 16KB: Returned to medium pool
//   - capacity ≤ 64KB: Returned to large pool
//   - capacity > 64KB: Discarded (not pooled)
//
// Important:
//   - Buffer is cleared (length set to 0) before pooling
//   - Capacity is preserved (underlying array kept)
//   - Oversized buffers (>64KB) are not pooled to prevent memory bloat
//   - nil buffers are safely ignored
//
// Thread safety: Safe for concurrent use by multiple goroutines.
//
// Best practice:
//   Always use defer to ensure buffers are returned:
//     buf := pool.Get(1024)
//     defer pool.Put(buf)
//
// Note: Caller must not use buffer after calling Put.
// The pool may hand the same buffer to another goroutine immediately.
func (bp *BufferPool) Put(buf *[]byte) {
	if buf == nil {
		return
	}

	size := cap(*buf)

	// Clear the buffer
	*buf = (*buf)[:0]

	switch {
	case size <= 4096:
		bp.small.Put(buf)
	case size <= 16384:
		bp.medium.Put(buf)
	case size <= 65536:
		bp.large.Put(buf)
	// Don't pool buffers larger than 64KB
	}
}
