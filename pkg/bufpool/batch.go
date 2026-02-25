//go:build linux

// Package bufpool provides utilities for batch network I/O
package bufpool

import (
	"net"
	"syscall"

	"golang.org/x/net/ipv4"
)

// BatchReader provides high-performance batch UDP reading using recvmmsg
type BatchReader struct {
	conn       *net.UDPConn
	packetConn *ipv4.PacketConn
	batchSize  int
	messages   []ipv4.Message
	buffers    []*Buffer
}

// NewBatchReader creates a new batch reader for the given UDP connection
func NewBatchReader(conn *net.UDPConn, batchSize int) *BatchReader {
	if batchSize <= 0 {
		batchSize = 64 // Default batch size
	}

	br := &BatchReader{
		conn:       conn,
		packetConn: ipv4.NewPacketConn(conn),
		batchSize:  batchSize,
		messages:   make([]ipv4.Message, batchSize),
		buffers:    make([]*Buffer, batchSize),
	}

	// Pre-allocate buffers
	for i := 0; i < batchSize; i++ {
		br.buffers[i] = NewBuffer(MediumBufferSize)
		br.messages[i].Buffers = [][]byte{br.buffers[i].Data}
	}

	return br
}

// ReadBatch reads multiple packets in a single system call
// Returns the number of messages read and any error
func (br *BatchReader) ReadBatch() (int, error) {
	// Reset buffers for reuse
	for i := 0; i < br.batchSize; i++ {
		br.messages[i].Buffers[0] = br.buffers[i].Data[:cap(br.buffers[i].Data)]
		br.messages[i].N = 0
	}

	n, err := br.packetConn.ReadBatch(br.messages, syscall.MSG_WAITFORONE)
	if err != nil {
		return 0, err
	}

	// Update buffer lengths based on actual read sizes
	for i := 0; i < n; i++ {
		br.buffers[i].Data = br.messages[i].Buffers[0][:br.messages[i].N]
	}

	return n, nil
}

// Message returns the i-th message from the last ReadBatch call
func (br *BatchReader) Message(i int) ([]byte, net.Addr) {
	if i < 0 || i >= br.batchSize {
		return nil, nil
	}
	return br.buffers[i].Data, br.messages[i].Addr
}

// Close releases all resources
func (br *BatchReader) Close() {
	for _, buf := range br.buffers {
		if buf != nil {
			buf.Release()
		}
	}
}

// BatchWriter provides high-performance batch UDP writing using sendmmsg
type BatchWriter struct {
	conn       *net.UDPConn
	packetConn *ipv4.PacketConn
	batchSize  int
	messages   []ipv4.Message
	pending    int
}

// NewBatchWriter creates a new batch writer for the given UDP connection
func NewBatchWriter(conn *net.UDPConn, batchSize int) *BatchWriter {
	if batchSize <= 0 {
		batchSize = 64
	}

	return &BatchWriter{
		conn:       conn,
		packetConn: ipv4.NewPacketConn(conn),
		batchSize:  batchSize,
		messages:   make([]ipv4.Message, batchSize),
		pending:    0,
	}
}

// Add adds a message to the batch
// Returns true if the batch is full and should be flushed
func (bw *BatchWriter) Add(data []byte, addr net.Addr) bool {
	if bw.pending >= bw.batchSize {
		return true
	}

	bw.messages[bw.pending].Buffers = [][]byte{data}
	bw.messages[bw.pending].Addr = addr
	bw.pending++

	return bw.pending >= bw.batchSize
}

// Flush sends all pending messages in a single system call
func (bw *BatchWriter) Flush() (int, error) {
	if bw.pending == 0 {
		return 0, nil
	}

	n, err := bw.packetConn.WriteBatch(bw.messages[:bw.pending], 0)
	bw.pending = 0
	return n, err
}

// Pending returns the number of pending messages
func (bw *BatchWriter) Pending() int {
	return bw.pending
}
