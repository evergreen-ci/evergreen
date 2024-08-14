package parameterstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSMClientImpl(t *testing.T) {
	assert.Implements(t, (*SSMClient)(nil), &ssmClientImpl{})
}
