package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandRegistry(t *testing.T) {
	assert := assert.New(t) // nolint

	r := newCommandRegistry()
	assert.NotNil(r.cmds)
	assert.NotNil(r.mu)
	assert.NotNil(evgRegistry)

	assert.Len(r.cmds, 0)

	factory := CommandFactory(func() Command { return nil })
	assert.NotNil(factory)
	assert.Error(r.registerCommand("", factory))
	assert.Len(r.cmds, 0)
	assert.Error(r.registerCommand("foo", nil))
	assert.Len(r.cmds, 0)

	assert.NoError(r.registerCommand("cmd.factory", factory))
	assert.Len(r.cmds, 1)
	assert.Error(r.registerCommand("cmd.factory", factory))
	assert.Len(r.cmds, 1)

	retFactory, ok := r.getCommandFactory("cmd.factory")
	assert.True(ok)
	assert.NotNil(retFactory)
}

func TestGlobalCommandRegistryNamesMatchExpectedValues(t *testing.T) {
	assert := assert.New(t) // nolint

	evgRegistry.mu.Lock()
	defer evgRegistry.mu.Unlock()
	for name, factory := range evgRegistry.cmds {
		cmd := factory()
		assert.Equal(name, cmd.Name())
	}
}
