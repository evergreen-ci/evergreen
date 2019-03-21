package job

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Counter is simple enough that a single simple test seems like
// sufficient coverage here.

func TestIDsAreIncreasing(t *testing.T) {
	assert := assert.New(t)

	for range [100]int{} {
		first := GetNumber()
		second := GetNumber()

		assert.True(first < second)
		assert.Equal(first+1, second)
	}
}
