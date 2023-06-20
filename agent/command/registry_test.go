package command

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandRegistry(t *testing.T) {
	assert := assert.New(t)

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
	assert := assert.New(t)

	evgRegistry.mu.Lock()
	defer evgRegistry.mu.Unlock()
	for name, factory := range evgRegistry.cmds {
		cmd := factory()
		assert.Equal(name, cmd.Name())
	}
}

func TestRenderCommands(t *testing.T) {
	registry := newCommandRegistry()
	registry.cmds = map[string]CommandFactory{
		"command.mock": func() Command { return &mockCommand{} },
	}

	t.Run("NoType", func(t *testing.T) {
		info := model.PluginCommandConf{Command: "command.mock"}
		project := model.Project{}

		cmds, err := registry.renderCommands(info, &project, BlockInfo{})
		assert.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, model.DefaultCommandType, cmds[0].Type())
	})

	t.Run("ProjectHasType", func(t *testing.T) {
		info := model.PluginCommandConf{Command: "command.mock"}
		project := model.Project{CommandType: evergreen.CommandTypeSetup}

		cmds, err := registry.renderCommands(info, &project, BlockInfo{})
		assert.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, evergreen.CommandTypeSetup, cmds[0].Type())
	})

	t.Run("CommandConfHasType", func(t *testing.T) {
		info := model.PluginCommandConf{
			Command: "command.mock",
			Type:    evergreen.CommandTypeSystem,
		}
		project := model.Project{}

		cmds, err := registry.renderCommands(info, &project, BlockInfo{})
		assert.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, evergreen.CommandTypeSystem, cmds[0].Type())
	})

	t.Run("ConfAndProjectHaveType", func(t *testing.T) {
		info := model.PluginCommandConf{
			Command: "command.mock",
			Type:    evergreen.CommandTypeSystem,
		}
		project := model.Project{CommandType: evergreen.CommandTypeSetup}

		cmds, err := registry.renderCommands(info, &project, BlockInfo{})
		assert.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, evergreen.CommandTypeSystem, cmds[0].Type())
	})
	t.Run("CommandHasExplicitDisplayName", func(t *testing.T) {
		const displayName = "potato"
		info := model.PluginCommandConf{
			Command:     "command.mock",
			DisplayName: displayName,
		}
		cmds, err := registry.renderCommands(info, &model.Project{}, BlockInfo{
			Block:     "pre",
			CmdNum:    5,
			TotalCmds: 8,
		})
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, displayName, cmds[0].DisplayName())
	})
	t.Run("CommandHasDefaultDisplayName", func(t *testing.T) {
		info := model.PluginCommandConf{
			Command: "command.mock",
		}
		cmds, err := registry.renderCommands(info, &model.Project{}, BlockInfo{
			Block:     "pre",
			CmdNum:    5,
			TotalCmds: 8,
		})
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, "'command.mock' (step 5 of 8) in block 'pre'", cmds[0].DisplayName())
	})
	t.Run("CommandHasDefaultDisplayNameWithNoBlockInfo", func(t *testing.T) {
		info := model.PluginCommandConf{
			Command: "command.mock",
		}
		cmds, err := registry.renderCommands(info, &model.Project{}, BlockInfo{})
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, "'command.mock'", cmds[0].DisplayName())
	})
}
