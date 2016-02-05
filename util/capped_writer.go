package util

import (
	"bytes"
	"errors"
)

// CappedWriter implements a buffer that stores up to MaxBytes bytes.
// Returns ErrBufferFull on overflowing writes
type CappedWriter struct {
	Buffer   *bytes.Buffer
	MaxBytes int
}

// ErrBufferFull indicates that a CappedWriter's bytes.Buffer has MaxBytes bytes.
var ErrBufferFull = errors.New("buffer full")

// Write writes to the buffer. An error is returned if the buffer is full.
func (cw *CappedWriter) Write(in []byte) (int, error) {
	remaining := cw.MaxBytes - cw.Buffer.Len()
	if len(in) <= remaining {
		return cw.Buffer.Write(in)
	}
	// fill up the remaining buffer and return an error
	n, _ := cw.Buffer.Write(in[:remaining])
	return n, ErrBufferFull
}

// IsFull indicates whether the buffer is full.
func (cw *CappedWriter) IsFull() bool {
	return cw.Buffer.Len() == cw.MaxBytes
}

// String return the contents of the buffer as a string.
func (cw *CappedWriter) String() string {
	return cw.Buffer.String()
}
