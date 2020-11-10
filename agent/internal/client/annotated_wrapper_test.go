package client

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
)

func TestAnnotate(t *testing.T) {
	assert := assert.New(t)
	taskID := "task1"
	logType := "type"
	sender, err := send.NewInternalLogger("", send.LevelInfo{Threshold: level.Info, Default: level.Info})
	assert.NoError(err)
	wrapper := newAnnotatedWrapper(taskID, logType, sender)
	logger := logging.MakeGrip(wrapper)

	logger.Info(message.Fields{
		"foo": "bar",
	})
	messageString := sender.GetMessage().Message.String()
	assert.Contains(messageString, "task_id='task1'")
	assert.Contains(messageString, "type='type'")
	assert.Contains(messageString, "foo='bar'")
}
