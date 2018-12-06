package commitq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusFunctionRegistry(t *testing.T) {
	assert := assert.New(t)

	action, err := GetStatusAction("asdf")
	assert.Nil(action)
	assert.Error(err)

	action, err = GetStatusAction("github")
	assert.NotNil(action)
	assert.NoError(err)
}
