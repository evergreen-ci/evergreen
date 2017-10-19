package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentImplementations(t *testing.T) {
	assert := assert.New(t) // nolint

	assert.Implements((*Environment)(nil), &envState{})
}
