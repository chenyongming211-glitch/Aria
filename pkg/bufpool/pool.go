// Package bufpool provides zero-allocation buffer pools for high-performance networking
package bufpool

import (
	"sync"
)

// Pool sizes for different use cases
const (
	SmallBufferSize  = 512   // For control messages, STUN, etc.
	MediumBufferSize = 1500  // Standard MTU
	LargeBufferSize  = 65535 // Maximum UDP packet
)

var (
	// SmallPool for control plane messages (< 512 bytes)
	SmallPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, SmallBufferSize)
			return &buf
		},
	}

	// MediumPool for standard network packets (MTU size)
	MediumPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, MediumBufferSize)
			return &buf
		},
	}

	// LargePool for jumbo frames or reassembly
	LargePool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, LargeBufferSize)
			return &buf
		},
	}
)

// GetSmall returns a small buffer from the pool
func GetSmall() *[]byte {
	return SmallPool.Get().(*[]byte)
}

// PutSmall returns a small buffer to the pool
func PutSmall(buf *[]byte) {
	if buf == nil || cap(*buf) < SmallBufferSize {
		return
	}
	*buf = (*buf)[:SmallBufferSize] // Reset length
	SmallPool.Put(buf)
}

// GetMedium returns a medium buffer from the pool
func GetMedium() *[]byte {
	return MediumPool.Get().(*[]byte)
}

// PutMedium returns a medium buffer to the pool
func PutMedium(buf *[]byte) {
	if buf == nil || cap(*buf) < MediumBufferSize {
		return
	}
	*buf = (*buf)[:MediumBufferSize]
	MediumPool.Put(buf)
}

// GetLarge returns a large buffer from the pool
func GetLarge() *[]byte {
	return LargePool.Get().(*[]byte)
}

// PutLarge returns a large buffer to the pool
func PutLarge(buf *[]byte) {
	if buf == nil || cap(*buf) < LargeBufferSize {
		return
	}
	*buf = (*buf)[:LargeBufferSize]
	LargePool.Put(buf)
}

// Buffer wraps a pooled buffer with automatic return
type Buffer struct {
	Data []byte
	pool *sync.Pool
	cap  int
}

// NewBuffer creates a buffer of the specified size from the appropriate pool
func NewBuffer(size int) *Buffer {
	switch {
	case size <= SmallBufferSize:
		buf := GetSmall()
		return &Buffer{Data: (*buf)[:size], pool: &SmallPool, cap: SmallBufferSize}
	case size <= MediumBufferSize:
		buf := GetMedium()
		return &Buffer{Data: (*buf)[:size], pool: &MediumPool, cap: MediumBufferSize}
	default:
		buf := GetLarge()
		return &Buffer{Data: (*buf)[:size], pool: &LargePool, cap: LargeBufferSize}
	}
}

// Release returns the buffer to its pool
func (b *Buffer) Release() {
	if b == nil || b.pool == nil {
		return
	}
	// Restore to original capacity before returning
	b.Data = b.Data[:b.cap]
	b.pool.Put(&b.Data)
	b.Data = nil
	b.pool = nil
}

// Bytes returns the underlying byte slice
func (b *Buffer) Bytes() []byte {
	return b.Data
}

// Len returns the length of the buffer
func (b *Buffer) Len() int {
	return len(b.Data)
}

// Reset resets the buffer length to zero (keeps capacity)
func (b *Buffer) Reset() {
	b.Data = b.Data[:0]
}

// Grow grows the buffer to accommodate n more bytes
func (b *Buffer) Grow(n int) {
	if len(b.Data)+n <= cap(b.Data) {
		b.Data = b.Data[:len(b.Data)+n]
	}
}
