package main // import "github.com/nutmegdevelopment/sumologic/filestream"

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/nutmegdevelopment/sumologic/buffer"
	"github.com/stretchr/testify/assert"
)

type testUploader struct{}

func (t *testUploader) Send(data []byte, name string) (err error) {
	if name != "filetest" {
		return errors.New("Bad name")
	}
	if bytes.Compare(data, []byte("test1\ntest2\ntest3")) != 0 {
		return errors.New("Bad data")
	}
	return
}

func TestWatchFile(t *testing.T) {
	sendName = "filetest"
	b := buffer.NewBuffer(8)
	f, err := ioutil.TempFile("/tmp", "filestream-test")
	assert.NoError(t, err)

	go func() {
		f.WriteString("test1\n")
		f.WriteString("test2\n")
		f.WriteString("test3\n")
		f.Sync()
		f.Close()
		time.Sleep(500 * time.Millisecond)
		os.Remove(f.Name())
	}()

	err = watchFile(b, f.Name())
	assert.NoError(t, err)

	err = b.Send(&testUploader{})
	assert.NoError(t, err)

	err = watchFile(b, f.Name())
	assert.Error(t, err)
}
