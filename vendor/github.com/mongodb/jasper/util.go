package jasper

import (
	"bytes"
	"sync"
)

type localBuffer struct {
	b bytes.Buffer
	sync.RWMutex
}

func (b *localBuffer) Read(p []byte) (n int, err error) {
	b.RLock()
	defer b.RUnlock()
	return b.b.Read(p)
}
func (b *localBuffer) Write(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	return b.b.Write(p)
}
func (b *localBuffer) String() string {
	b.RLock()
	defer b.RUnlock()
	return b.b.String()
}

func (b *localBuffer) Close() error { return nil }
