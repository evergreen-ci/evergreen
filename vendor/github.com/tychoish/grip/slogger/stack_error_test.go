package slogger

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tychoish/grip/message"
)

func TestSlackError(t *testing.T) {
	assert := assert.New(t)

	msg := NewStackError("foo %s", "bar")
	assert.Implements((*message.Composer)(nil), msg)
	assert.Implements((*error)(nil), msg)
	assert.True(len(msg.Stacktrace) > 0)

	assert.True(strings.HasPrefix(msg.Resolve(), "foo bar"))

	assert.Equal("", NewStackError("").Resolve())

	assert.Equal(NewStackError("").Resolve(), NewStackError("").Error())
	assert.Equal(msg.Resolve(), msg.Error())

	// the raw structure always has stack data, generated in the
	// constructor, even if the message is nil.
	assert.True(len(fmt.Sprintf("%v", NewStackError("").Raw())) > 10)
}
