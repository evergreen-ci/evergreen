package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Counter is simple enough that a single simple test seems like
// sufficient coverage here.

func TestIDsAreIncreasing(t *testing.T) {
	assert := assert.New(t)

	for i := 0; i < 100; i++ {
		first := getNumber()
		second := getNumber()

		assert.True(first < second)
		assert.Equal(first+1, second)
	}
}
