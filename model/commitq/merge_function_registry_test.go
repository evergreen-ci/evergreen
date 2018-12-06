package commitq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeFunctionRegistry(t *testing.T) {
	assert := assert.New(t)

	action, err := GetMergeAction("asdf")
	assert.Nil(action)
	assert.Error(err)

	action, err = GetMergeAction("merge")
	assert.NotNil(action)
	assert.NoError(err)

	action, err = GetMergeAction("squash")
	assert.NotNil(action)
	assert.NoError(err)

	action, err = GetMergeAction("rebase")
	assert.NotNil(action)
	assert.NoError(err)
}
