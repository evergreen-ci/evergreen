package recovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPanicStringConverter(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("", panicString(nil))
	assert.Equal("foo", panicString("foo"))
}
