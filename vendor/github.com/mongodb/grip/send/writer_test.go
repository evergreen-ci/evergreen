package send

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenderWriter(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	sink, err := NewInternalLogger("sink", LevelInfo{level.Debug, level.Debug})
	assert.NoError(err)

	ws := NewWriterSender(sink)
	assert.Equal(ws.buffer.Len(), 0)

	// writing something without a new line character will cause it to not send.
	msg := []byte("hello world")
	n, err := ws.Write(msg)
	assert.NoError(err)
	assert.Equal(n, len(msg))
	assert.Equal(ws.buffer.Len(), n)
	assert.False(sink.HasMessage())

	newLine := []byte{'\n'}
	// if we add a new line character, then it'll flush
	n, err = ws.Write(newLine)
	assert.NoError(err)
	assert.Equal(n, len(newLine))
	assert.Equal(ws.buffer.Len(), 12)

	assert.NoError(ws.doSend())

	require.True(sink.HasMessage())
	m := sink.GetMessage()
	assert.True(m.Logged)
	assert.Equal(m.Message.String(), "hello world")
	// the above trimmed the final new line off, which is correct,
	// given how senders will actually newline deelimit messages anyway.
	//
	// at the same time, we should make sure that we preserve newlines internally
	msg = []byte("hello world\nhello grip\n")
	n, err = ws.Write(msg)
	assert.NoError(err)
	assert.Equal(n, len(msg))
	assert.Equal(ws.buffer.Len(), len(msg))

	assert.NoError(ws.doSend())

	assert.True(sink.HasMessage())
	assert.Equal(sink.Len(), 2)
	m = sink.GetMessage()
	m2 := sink.GetMessage()
	assert.True(m.Logged)
	assert.True(m2.Logged)
	assert.Equal(m.Message.String(), "hello world")
	assert.Equal(m2.Message.String(), "hello grip")

	// send a message, but no new line, means it lives in the buffer.
	msg = []byte("hello world")
	n, err = ws.Write(msg)
	assert.NoError(err)
	assert.Equal(n, len(msg))
	assert.Equal(ws.buffer.Len(), n)
	assert.False(sink.HasMessage())

	assert.NotEqual(ws.buffer.Len(), 0)
	assert.NoError(ws.Close())
	assert.True(sink.HasMessage())
	m = sink.GetMessage()
	assert.True(m.Logged)
	assert.Equal(m.Message.String(), "hello world")
	numMessages := sink.Len()
	assert.Equal(ws.buffer.Len(), 0)
	assert.Equal(numMessages, sink.Len())

	for i := 0; i < 10; i++ {
		assert.NoError(ws.Close())
		assert.False(sink.GetMessage().Logged)
	}

	assert.Equal(ws.buffer.Len(), 0)
}
