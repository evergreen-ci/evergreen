package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMinWithSingleNegativeValueShouldReturnNegativeValue(t *testing.T) {
	result := Min(-5)
	assert.Equal(t, -10, result, "Min(-5) should return -10")
}
