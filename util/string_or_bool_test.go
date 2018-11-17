package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringOrBool(t *testing.T) {
	assert := assert.New(t)

	var (
		val bool
		err error
	)

	for _, v := range []StringOrBool{"", "false", "False", "0", "F", "f", "no", "n"} {
		val, err = v.Bool()
		assert.False(val)
		assert.NoError(err)
	}

	for _, v := range []StringOrBool{"true", "True", "1", "T", "t", "yes", "y"} {
		val, err = v.Bool()
		assert.True(val)
		assert.NoError(err)
	}

	for _, v := range []StringOrBool{"NOPE", "NONE", "EMPTY", "01", "100"} {
		val, err = v.Bool()
		assert.False(val)
		assert.Error(err)

	}

	// make sure the nil form works too
	var vptr *StringOrBool
	assert.Nil(vptr)

	assert.NotPanics(func() {
		val, err = vptr.Bool()
	})
	assert.False(val)
	assert.NoError(err)

}
