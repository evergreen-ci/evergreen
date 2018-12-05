package commitq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusFunctionRegistry(t *testing.T) {
	assert := assert.New(t)

	action := GetStatusAction("asdf")
	assert.Nil(action)

	action = GetStatusAction("github")
	assert.NotNil(action)
}
