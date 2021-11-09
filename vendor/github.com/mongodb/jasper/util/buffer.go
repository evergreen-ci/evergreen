package util

import (
	"bytes"
	"sync"
)

// NewLocalBuffer returns a thread-safe Read/Write closer.
func NewLocalBuffer(b bytes.Buffer) *LocalBuffer { return &LocalBuffer{buffer: b} }

// LocalBuffer provides a thread-safe in-memory buffer.
type LocalBuffer struct {
	buffer bytes.Buffer
	sync.RWMutex
}

// Read performs a thread-safe in-memory read.
func (b *LocalBuffer) Read(p []byte) (n int, err error) {
	b.RLock()
	defer b.RUnlock()
	return b.buffer.Read(p)
}

// Write performs a thread-safe in-memory write.
func (b *LocalBuffer) Write(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	return b.buffer.Write(p)
}

// String returns the in-memory buffer contents as a string in a thread-safe
// manner.
func (b *LocalBuffer) String() string {
	b.RLock()
	defer b.RUnlock()
	return b.buffer.String()
}

// Close is a no-op to satisfy the closer interface..
func (b *LocalBuffer) Close() error { return nil }
