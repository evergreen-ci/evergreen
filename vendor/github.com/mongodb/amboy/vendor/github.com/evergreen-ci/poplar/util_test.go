package poplar

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Utils for tests

func getPathOfFile() string {
	_, file, _, _ := runtime.Caller(1)
	return file
}

////////////////////////////////////////////////////////////////////////
//
// Tests of Util Functions

func TestBoolCheck(t *testing.T) {
	assert.False(t, isMoreThanOneTrue([]bool{}))
	assert.False(t, isMoreThanOneTrue([]bool{false}))
	assert.False(t, isMoreThanOneTrue([]bool{true}))
	assert.False(t, isMoreThanOneTrue([]bool{false, false}))
	assert.False(t, isMoreThanOneTrue([]bool{false, true}))
	assert.False(t, isMoreThanOneTrue([]bool{true, false}))
	assert.True(t, isMoreThanOneTrue([]bool{true, true}))
	assert.False(t, isMoreThanOneTrue([]bool{false, false, true}))
	assert.True(t, isMoreThanOneTrue([]bool{true, false, true}))
}
