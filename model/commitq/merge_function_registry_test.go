package commitq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeFunctionRegistry(t *testing.T) {
	assert := assert.New(t)

	action := GetMergeAction("asdf")
	assert.Nil(action)

	action = GetMergeAction("merge")
	assert.NotNil(action)

	action = GetMergeAction("squash")
	assert.NotNil(action)

	action = GetMergeAction("rebase")
	assert.NotNil(action)
}
