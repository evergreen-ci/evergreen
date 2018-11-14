package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringOrInt(t *testing.T) {
	assert := assert.New(t)

	var (
		i   int
		err error
	)

	for k, v := range map[StringOrInt]int{
		"-1000000": -1000000,
		"-1":       -1,
		"0":        0,
		"1":        1,
		"2":        2,
		"27017":    27017,
	} {
		i, err = k.Int()
		assert.NoError(err)
		assert.Equal(i, v)
	}
}
