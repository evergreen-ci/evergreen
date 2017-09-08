package client

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestRestClientLogSenderMessageContents(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := NewMock("url")
	td := TaskData{ID: "task", Secret: "secret"}
	s, ok := newLogSender(ctx, comm, "testStream", td).(*logSender)
	assert.True(ok)

	msgs := comm.GetMockMessages()
	assert.Len(msgs, 0)
	assert.Len(msgs["task"], 0)

	s.Send(message.NewDefaultMessage(level.Error, "hello world"))
	time.Sleep(bufferTime + time.Second)
	msgs = comm.GetMockMessages()
	assert.Len(msgs, 1)
	assert.Len(msgs["task"], 1)

	m := msgs["task"]
	assert.Equal("testStream", m[0].Type)
	assert.Equal(apimodels.LogErrorPrefix, m[0].Severity)
	assert.Equal("hello world", m[0].Message)
}

func TestRestClientLogSenderDoesNotLogInErrorConditions(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := NewMock("url")
	comm.loggingShouldFail = true
	td := TaskData{ID: "task", Secret: "secret"}
	s, ok := newLogSender(ctx, comm, "testStream", td).(*logSender)
	assert.True(ok)

	s.Send(message.NewDefaultMessage(level.Error, "hello world!!"))
	time.Sleep(bufferTime + time.Second)

	msgs := comm.GetMockMessages()
	assert.Len(msgs, 0)
	assert.Len(msgs["task"], 0)
}

func TestLevelrConverters(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	assert.Equal("UNKNOWN", priorityToString(level.Invalid))
	assert.Equal("UNKNOWN", priorityToString(level.Priority(2)))

	assert.Equal(apimodels.LogDebugPrefix, priorityToString(level.Debug))
	assert.Equal(apimodels.LogDebugPrefix, priorityToString(level.Trace))
	assert.Equal(apimodels.LogInfoPrefix, priorityToString(level.Info))
	assert.Equal(apimodels.LogInfoPrefix, priorityToString(level.Notice))
	assert.Equal(apimodels.LogWarnPrefix, priorityToString(level.Warning))
	assert.Equal(apimodels.LogErrorPrefix, priorityToString(level.Error))
	assert.Equal(apimodels.LogErrorPrefix, priorityToString(level.Alert))
	assert.Equal(apimodels.LogErrorPrefix, priorityToString(level.Critical))
	assert.Equal(apimodels.LogErrorPrefix, priorityToString(level.Emergency))
}
