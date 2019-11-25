package mock

import (
	"testing"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
)

func TestMockInterfaces(t *testing.T) {
	assert.Implements(t, (*jasper.Manager)(nil), &Manager{})
	assert.Implements(t, (*jasper.Process)(nil), &Process{})
	assert.Implements(t, (*jasper.RemoteClient)(nil), &RemoteClient{})
}
