package websocket

import (
	"sync"
	"unsafe"
)

// MessageBuffer is a reusable buffer for zero-allocation message processing
type MessageBuffer struct {
	data []byte
	pos  int
}

// MessagePool manages reusable message buffers with size classes
type MessagePool struct {
	small  sync.Pool // 256 bytes
	medium sync.Pool // 1KB
	large  sync.Pool // 4KB
}

var globalMessagePool = &MessagePool{
	small: sync.Pool{
		New: func() interface{} {
			return &MessageBuffer{
				data: make([]byte, 256),
				pos:  0,
			}
		},
	},
	medium: sync.Pool{
		New: func() interface{} {
			return &MessageBuffer{
				data: make([]byte, 1024),
				pos:  0,
			}
		},
	},
	large: sync.Pool{
		New: func() interface{} {
			return &MessageBuffer{
				data: make([]byte, 4096),
				pos:  0,
			}
		},
	},
}

// Get returns a buffer of appropriate size
func (p *MessagePool) Get(size int) *MessageBuffer {
	var buf *MessageBuffer
	switch {
	case size <= 256:
		buf = p.small.Get().(*MessageBuffer)
	case size <= 1024:
		buf = p.medium.Get().(*MessageBuffer)
	default:
		buf = p.large.Get().(*MessageBuffer)
	}
	buf.pos = 0
	return buf
}

// Put returns a buffer to the pool
func (p *MessagePool) Put(buf *MessageBuffer) {
	if buf == nil {
		return
	}

	// Clear sensitive data
	for i := range buf.data[:buf.pos] {
		buf.data[i] = 0
	}
	buf.pos = 0

	switch cap(buf.data) {
	case 256:
		p.small.Put(buf)
	case 1024:
		p.medium.Put(buf)
	case 4096:
		p.large.Put(buf)
	}
}

// Write appends data to the buffer
func (b *MessageBuffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Ensure capacity
	if b.pos+len(p) > cap(b.data) {
		// For simplicity, we don't grow - use a larger buffer from pool
		return 0, nil
	}

	n = copy(b.data[b.pos:], p)
	b.pos += n
	return n, nil
}

// Bytes returns the written bytes without allocation
func (b *MessageBuffer) Bytes() []byte {
	return b.data[:b.pos]
}

// Reset clears the buffer for reuse
func (b *MessageBuffer) Reset() {
	b.pos = 0
}

// FastString converts bytes to string without allocation using unsafe
func FastString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// FastBytes converts string to bytes without allocation using unsafe
func FastBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}