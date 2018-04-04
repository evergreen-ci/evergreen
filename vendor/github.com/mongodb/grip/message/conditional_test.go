package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConditionalMessage(t *testing.T) {
	assert := assert.New(t) // nolint

	comp := When(true, "foo")
	assert.True(comp.Loggable())

	comp = When(false, "foo")
	assert.False(comp.Loggable())
	comp = When(true, "")
	assert.False(comp.Loggable(), "%T: %s", comp.(*condComposer).msg, comp.(*condComposer).msg)

	comp = Whenln(true, "foo", "bar")
	assert.True(comp.Loggable())
	comp = Whenln(false, "foo", "bar")
	assert.False(comp.Loggable())
	comp = Whenln(true, "", "")
	assert.False(comp.Loggable(), "%T: %s", comp.(*condComposer).msg, comp.(*condComposer).msg)

	comp = Whenf(true, "f%soo", "bar")
	assert.True(comp.Loggable())
	comp = Whenf(false, "f%soo", "bar")
	assert.False(comp.Loggable())
	comp = Whenf(true, "", "foo")
	assert.False(comp.Loggable(), "%T: %s", comp.(*condComposer).msg, comp.(*condComposer).msg)

	comp = WhenMsg(true, "foo")
	assert.True(comp.Loggable())
	comp = WhenMsg(false, "bar")
	assert.False(comp.Loggable())
}
