package main // import "github.com/nutmegdevelopment/sumologic/journalstream"

import (
	"testing"
	"time"

	"github.com/coreos/go-systemd/journal"
	"github.com/coreos/go-systemd/sdjournal"
	"github.com/fortytw2/leaktest"
	"github.com/nutmegdevelopment/sumologic/buffer"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

type testUploader struct {
	data []byte
	name string
}

func (t *testUploader) Send(data []byte, name string) (err error) {
	t.data = data
	t.name = name
	return
}

func TestWatch(t *testing.T) {
	// Check for leaks
	defer leaktest.Check(t)()

	quitCh := make(chan bool, 1)
	eventCh := make(chan *sdjournal.JournalEntry, 1024)

	go watch(eventCh, quitCh)

	id := "journalstream-test-" + uuid.NewV1().String()

	err := journal.Send(id, journal.PriInfo, map[string]string{})
	assert.NoError(t, err)

	tstart := time.Now()

	var success bool

	for e := range eventCh {
		if m, ok := e.Fields["MESSAGE"]; ok {
			if m == id {
				success = true
				break
			}
		}
		if time.Since(tstart) >= 15*time.Second {
			break
		}
	}
	quitCh <- true

	assert.True(t, success)
}

func TestParse(t *testing.T) {
	quitCh := make(chan bool, 1)
	eventCh := make(chan *sdjournal.JournalEntry, 1024)
	buf := buffer.NewBuffer(bSize)

	nameField = "JOURNALD_TEST"
	window = 10

	go parse(eventCh, buf, quitCh)

	// Normal message

	// Round to usec here to avoid potential rounding errors later
	tstamp := time.Now().Round(time.Microsecond)

	eventCh <- &sdjournal.JournalEntry{
		RealtimeTimestamp: uint64(tstamp.UnixNano() / (int64(time.Microsecond) / int64(time.Nanosecond))),
		Fields: map[string]string{
			"JOURNALD_TEST": "test-unit",
			"MESSAGE":       "test-message",
		},
	}

	time.Sleep(100 * time.Millisecond)
	u := new(testUploader)
	buf.Send(u)

	assert.Contains(t, string(u.data), "test-message")
	assert.Contains(t, string(u.data), tstamp.String())
	assert.Equal(t, "test-unit", u.name)

	// Missing name field

	tstamp = time.Now().Round(time.Microsecond)

	eventCh <- &sdjournal.JournalEntry{
		RealtimeTimestamp: uint64(tstamp.UnixNano() / (int64(time.Microsecond) / int64(time.Nanosecond))),
		Fields: map[string]string{
			"JOURNALD_FAIL": "test-unit",
			"MESSAGE":       "test-message",
		},
	}

	time.Sleep(100 * time.Millisecond)
	u = new(testUploader)
	buf.Send(u)

	assert.Contains(t, string(u.data), "test-message")
	assert.Contains(t, string(u.data), tstamp.String())
	assert.Equal(t, "UNDEFINED", u.name)

	// Missing MESSAGE

	tstamp = time.Now().Round(time.Microsecond)

	eventCh <- &sdjournal.JournalEntry{
		RealtimeTimestamp: uint64(tstamp.UnixNano() / (int64(time.Microsecond) / int64(time.Nanosecond))),
		Fields: map[string]string{
			"JOURNALD_TEST": "test-unit",
			"FAIL":          "test-message",
		},
	}

	time.Sleep(100 * time.Millisecond)
	u = new(testUploader)
	buf.Send(u)

	assert.Empty(t, u.data)
	assert.Empty(t, u.name)

	// Old message

	tstamp = time.Now().Add(-time.Minute).Round(time.Microsecond)

	eventCh <- &sdjournal.JournalEntry{
		RealtimeTimestamp: uint64(tstamp.UnixNano() / (int64(time.Microsecond) / int64(time.Nanosecond))),
		Fields: map[string]string{
			"JOURNALD_TEST": "test-unit",
			"FAIL":          "test-message",
		},
	}

	time.Sleep(100 * time.Millisecond)
	u = new(testUploader)
	buf.Send(u)

	assert.Empty(t, u.data)
	assert.Empty(t, u.name)

}
