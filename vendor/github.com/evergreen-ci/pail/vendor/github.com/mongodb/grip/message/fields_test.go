package message

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestFieldsLevelMutability(t *testing.T) {
	assert := assert.New(t) // nolint

	m := Fields{"message": "hello world"}
	c := ConvertToComposer(level.Error, m)

	r := c.Raw().(Fields)
	assert.Equal(level.Error, c.Priority())
	assert.Equal(level.Error, r["metadata"].(*Base).Level)

	c = ConvertToComposer(level.Info, m)
	r = c.Raw().(Fields)
	assert.Equal(level.Info, c.Priority())
	assert.Equal(level.Info, r["metadata"].(*Base).Level)
}
