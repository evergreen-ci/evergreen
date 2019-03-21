package slogger

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

func TestDevNull(t *testing.T) {
	devNull, err := DevNullAppender()
	assert.NoError(t, err)
	assert.NoError(t, devNull.SetErrorHandler(func(err error, c message.Composer) {
		assert.Fail(t, "Send() should not fail for DevNullAppender()")
	}))

	devNull.Send(message.NewDefaultMessage(level.Info, "foobar"))
}
