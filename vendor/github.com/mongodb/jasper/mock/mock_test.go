package mock

import (
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/remote"
	"github.com/stretchr/testify/assert"
)

func TestMockInterfaces(t *testing.T) {
	assert.Implements(t, (*jasper.Manager)(nil), &Manager{})
	assert.Implements(t, (*jasper.Process)(nil), &Process{})
	assert.Implements(t, (*remote.Manager)(nil), &RemoteManager{})
	assert.Implements(t, (*jasper.LoggingCache)(nil), &LoggingCache{})
	assert.Implements(t, (*jasper.OOMTracker)(nil), &OOMTracker{})
}
