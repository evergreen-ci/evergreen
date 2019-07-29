package jasper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMockInterfaces(t *testing.T) {
	manager := &MockManager{}
	_, ok := interface{}(manager).(Manager)
	assert.True(t, ok)

	process := &MockProcess{}
	_, ok = interface{}(process).(Process)
	assert.True(t, ok)

	remoteClient := &MockRemoteClient{}
	_, ok = interface{}(remoteClient).(RemoteClient)
	assert.True(t, ok)
}
