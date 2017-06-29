package client

import (
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

func TestRestClientLogSnder(t *testing.T) {
	assert := assert.New(t)

	comm := NewMockEvergreenREST().(*MockEvergreenREST)
	s := newLogSender(comm, "testStream", "task", "secret")

	assert.Len(comm.logMessages, 0)
	assert.Len(comm.logMessages["task"], 0)

	s.Send(message.NewDefaultMessage(level.Error, "hello world"))
	assert.Len(comm.logMessages, 1)
	assert.Len(comm.logMessages["task"], 1)

	// test that we don't log in error conditions
	comm.loggingShouldFail = true
	s.Send(message.NewDefaultMessage(level.Error, "hello world!!"))
	assert.Len(comm.logMessages, 1)
	assert.Len(comm.logMessages["task"], 1)
	comm.loggingShouldFail = false

	// test the contents of the message
	m := comm.logMessages["task"][0]
	assert.Equal("testStream", m.Type)
	assert.Equal(apimodels.LogErrorPrefix, m.Severity)
	assert.Equal("hello world", m.Message)

	// test composer conversionss
	cs := []message.Composer{
		message.NewString("hello"),
		message.NewString("hello"),
		message.MakeGroupComposer(message.NewString("hello"), message.NewString("hello"),
			message.MakeGroupComposer(message.NewString("hello"), message.NewString("hello"))),
	}

	lsender := s.(*logSender)

	assert.Len(cs, 3)
	lms := lsender.convertMessages(message.NewGroupComposer(cs))
	assert.Len(lms, 6)

	lsender.Send(message.NewGroupComposer(cs))
	assert.Len(comm.logMessages, 1)
	assert.Len(comm.logMessages["task"], 7)

}

func TestLevelrConverters(t *testing.T) {
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
