package slogger

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

func TestSlackError(t *testing.T) {
	assert := assert.New(t)

	msg := NewStackError("foo %s", "bar")
	assert.Implements((*message.Composer)(nil), msg)
	assert.Implements((*error)(nil), msg)
	assert.True(len(msg.Stacktrace) > 0)

	assert.True(strings.HasPrefix(msg.String(), "foo bar"))

	assert.Equal("", NewStackError("").message)

	assert.Equal(NewStackError("").String(), NewStackError("").Error())
	assert.Equal(msg.String(), msg.Error())

	// the raw structure always has stack data, generated in the
	// constructor, even if the message is nil.
	assert.True(len(fmt.Sprintf("%v", NewStackError("").Raw())) > 10)
}
