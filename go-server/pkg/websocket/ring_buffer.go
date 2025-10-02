package websocket

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

const (
	// RingBufferSize must be power of 2 for fast modulo
	RingBufferSize = 16384 // 16K slots
	RingBufferMask = RingBufferSize - 1
)

// RingBuffer is a lock-free multi-producer single-consumer ring buffer
type RingBuffer struct {
	// Cache line padding to prevent false sharing
	_ [64]byte
	head uint64 // Producer position (atomic)
	_ [64]byte
	tail uint64 // Consumer position (atomic)
	_ [64]byte

	slots [RingBufferSize]unsafe.Pointer
}

// NewRingBuffer creates a new lock-free ring buffer
func NewRingBuffer() *RingBuffer {
	return &RingBuffer{}
}

// Push adds a message to the ring buffer (lock-free)
func (rb *RingBuffer) Push(msg []byte) bool {
	// Claim a slot
	head := atomic.AddUint64(&rb.head, 1) - 1
	tail := atomic.LoadUint64(&rb.tail)

	// Check if buffer is full
	if head-tail >= RingBufferSize {
		return false
	}

	// Copy message to avoid holding reference
	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)

	// Store in slot
	slot := head & RingBufferMask
	atomic.StorePointer(&rb.slots[slot], unsafe.Pointer(&msgCopy))

	return true
}

// Pop removes and returns a message from the ring buffer
func (rb *RingBuffer) Pop() []byte {
	tail := atomic.LoadUint64(&rb.tail)
	head := atomic.LoadUint64(&rb.head)

	// Check if empty
	if tail >= head {
		return nil
	}

	// Get slot
	slot := tail & RingBufferMask
	msgPtr := atomic.LoadPointer(&rb.slots[slot])

	if msgPtr == nil {
		// Slot not ready yet, spin wait
		runtime.Gosched()
		return nil
	}

	// Extract message
	msg := *(*[]byte)(msgPtr)

	// Clear slot and advance tail
	atomic.StorePointer(&rb.slots[slot], nil)
	atomic.StoreUint64(&rb.tail, tail+1)

	return msg
}

// Size returns approximate number of messages in buffer
func (rb *RingBuffer) Size() int {
	head := atomic.LoadUint64(&rb.head)
	tail := atomic.LoadUint64(&rb.tail)

	if head >= tail {
		return int(head - tail)
	}
	return 0
}

// BroadcastBuffer manages per-client broadcast buffers
type BroadcastBuffer struct {
	buffers map[*Client]*RingBuffer
}

// NewBroadcastBuffer creates a broadcast buffer manager
func NewBroadcastBuffer() *BroadcastBuffer {
	return &BroadcastBuffer{
		buffers: make(map[*Client]*RingBuffer),
	}
}

// AddClient adds a client with its own ring buffer
func (bb *BroadcastBuffer) AddClient(client *Client) {
	bb.buffers[client] = NewRingBuffer()
}

// RemoveClient removes a client's buffer
func (bb *BroadcastBuffer) RemoveClient(client *Client) {
	delete(bb.buffers, client)
}

// Broadcast sends message to all client buffers (lock-free)
func (bb *BroadcastBuffer) Broadcast(msg []byte) {
	for _, buffer := range bb.buffers {
		buffer.Push(msg) // Non-blocking push
	}
}