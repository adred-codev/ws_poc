package main

import (
	"sync"
)

// BufferPool manages a pool of reusable byte buffers
type BufferPool struct {
	small  sync.Pool // 4KB buffers
	medium sync.Pool // 16KB buffers
	large  sync.Pool // 64KB buffers
}

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
