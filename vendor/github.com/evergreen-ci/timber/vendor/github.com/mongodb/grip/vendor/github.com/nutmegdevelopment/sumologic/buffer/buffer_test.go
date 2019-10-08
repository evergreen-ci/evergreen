package buffer // import "github.com/nutmegdevelopment/sumologic/buffer"

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testUploader struct{}

func (t *testUploader) Send(data []byte, name string) (err error) {
	if data == nil || name == "" || len(data) == 0 {
		err = errors.New("Empty input")
	}
	return
}

func TestBuffer(t *testing.T) {

	// Set a small buffer size so we can quickly overflow it.
	buf := NewBuffer(8)

	for i := 0; i < 128; i++ {
		data := strconv.AppendInt([]byte{}, int64(i), 10)
		buf.Add(data, "test")
	}

	assert.Equal(t, "test", buf.names[0])
	assert.Equal(t, []byte("0"), buf.data[0])

	uploader := new(testUploader)

	err := buf.Send(uploader)
	assert.NoError(t, err)
	assert.Len(t, buf.data, 0)

	for i := 0; i < 128; i++ {
		data := strconv.AppendInt([]byte{}, int64(i), 10)
		buf.Add(data, "")
	}

	err = buf.Send(uploader)
	assert.Error(t, err)
	assert.Len(t, buf.data, 128)
}

// This aggressively calls Add and Send, hopefully triggering any race conditions.
func TestBufferRace(t *testing.T) {
	buf := NewBuffer(8)
	q1 := make(chan bool)
	q2 := make(chan bool)

	go func() {
		in := []byte("Some test data here")
		for {
			select {
			case <-q1:
				return
			default:
				buf.Add(in, "test1")
				time.Sleep(50 * time.Microsecond)
			}
		}
	}()

	go func() {
		in := []byte("Some test data here")
		for {
			select {
			case <-q2:
				return
			default:
				buf.Add(in, "test2")
				time.Sleep(50 * time.Microsecond)
			}
		}
	}()

	uploader := new(testUploader)

	for i := 0; i < 10; i++ {
		buf.Send(uploader)

		time.Sleep(10 * time.Millisecond)
	}

	q1 <- true
	q2 <- true
}
