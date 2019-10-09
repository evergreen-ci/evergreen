package mock

import (
	"testing"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
)

func TestMockInterfaces(t *testing.T) {
	manager := &Manager{}
	_, ok := interface{}(manager).(jasper.Manager)
	assert.True(t, ok)

	process := &Process{}
	_, ok = interface{}(process).(jasper.Process)
	assert.True(t, ok)

	remoteClient := &RemoteClient{}
	_, ok = interface{}(remoteClient).(jasper.RemoteClient)
	assert.True(t, ok)
}
