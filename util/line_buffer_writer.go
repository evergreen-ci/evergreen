package util

import (
	"bytes"
	"io"
	"sync"
)

//LineBufferingWriter is an implementation of io.Writer that sends the newline
//delimited subslices of its inputs to a wrapped io.Writer, buffering as needed.
type LineBufferingWriter struct {
	buf           []byte
	wrappedWriter io.Writer
	bufferLock    sync.Mutex
}

func NewLineBufferingWriter(wrapped io.Writer) *LineBufferingWriter {
	return &LineBufferingWriter{
		buf:           []byte{},
		wrappedWriter: wrapped,
	}
}

// writes whatever is in the buffer out using the wrapped io.Writer
func (lbw *LineBufferingWriter) Flush() error {
	lbw.bufferLock.Lock()
	defer lbw.bufferLock.Unlock()
	if len(lbw.buf) == 0 {
		return nil
	}

	_, err := lbw.wrappedWriter.Write(lbw.buf)
	if err != nil {
		return err
	}
	lbw.buf = []byte{}
	return nil
}

// use the wrapped io.Writer to write anything that is delimited with a newline
func (lbw *LineBufferingWriter) Write(p []byte) (n int, err error) {
	lbw.bufferLock.Lock()
	defer lbw.bufferLock.Unlock()

	// Check to see if sum of two buffers is greater than 4K bytes
	if len(p)+len(lbw.buf) >= (4*1024) && len(lbw.buf) > 0 {
		_, err := lbw.wrappedWriter.Write(lbw.buf)
		if err != nil {
			return 0, err
		}
		lbw.buf = []byte{}
	}

	fullBuffer := append(lbw.buf, p...)
	lines := bytes.Split(fullBuffer, []byte{'\n'})
	for idx, val := range lines {
		if idx == len(lines)-1 {
			lbw.buf = val
		} else {
			_, err := lbw.wrappedWriter.Write(val)
			if err != nil {
				return 0, err
			}
		}
	}

	return len(p), nil
}
