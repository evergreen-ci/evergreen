package uglytests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPassingButInAnotherFile(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	assert.True(true)
}
func TestFailingButInAnotherFile(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	assert.True(false)
}
