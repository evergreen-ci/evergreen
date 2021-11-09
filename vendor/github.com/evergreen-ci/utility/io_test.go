package utility

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNopWriteCloser(t *testing.T) {
	wc := &myWriteCloser{}
	nop := NopWriteCloser(wc)
	nop.Close()
	assert.False(t, wc.closed)

	message := []byte("message")
	_, err := nop.Write(message)
	assert.Equal(t, message, wc.contents)
	assert.NoError(t, err)
}

type myWriteCloser struct {
	closed   bool
	contents []byte
}

func (w *myWriteCloser) Close() {
	w.closed = true
}
func (w *myWriteCloser) Write(p []byte) (n int, err error) {
	w.contents = append(w.contents, p...)
	return 0, nil
}
