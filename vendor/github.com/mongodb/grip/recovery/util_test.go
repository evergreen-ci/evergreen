package recovery

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestPanicStringConverter(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("", panicString(nil))
	assert.Equal("foo", panicString("foo"))
	assert.Equal("foo", panicString(errors.New("foo")))
}

func TestPanicErrorHandler(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(panicError(nil))
	assert.Error(panicError("foo"))
	assert.Error(panicError(""))
}
