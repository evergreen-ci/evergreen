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
		"command.mock": MockCommandFactory,
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
		assert.Equal(t, "'command.mock' ('potato') (step 5 of 8) in block 'pre'", cmds[0].FullDisplayName())
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
		assert.Equal(t, "'command.mock' (step 5 of 8) in block 'pre'", cmds[0].FullDisplayName())
	})
	t.Run("CommandHasDefaultDisplayNameWithNoBlockInfo", func(t *testing.T) {
		info := model.PluginCommandConf{
			Command: "command.mock",
		}
		cmds, err := registry.renderCommands(info, &model.Project{}, BlockInfo{})
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, "'command.mock'", cmds[0].FullDisplayName())
	})
	t.Run("CommandsInFuncHaveExplicitOrDefaultDisplayNames", func(t *testing.T) {
		info := model.PluginCommandConf{
			Function: "my-func",
		}
		p := &model.Project{
			Functions: map[string]*model.YAMLCommandSet{
				"my-func": {
					MultiCommand: []model.PluginCommandConf{
						{
							Command: "command.mock",
						},
						{
							Command: "command.mock",
						},
						{
							Command:     "command.mock",
							DisplayName: "run-a-shell-thing",
						},
					},
				},
			},
		}
		cmds, err := registry.renderCommands(info, p, BlockInfo{
			Block:     "pre",
			CmdNum:    1,
			TotalCmds: 1,
		})
		require.NoError(t, err)
		require.Len(t, cmds, 3)
		assert.Equal(t, "'command.mock' in function 'my-func' (step 1.1 of 1) in block 'pre'", cmds[0].FullDisplayName())
		assert.Equal(t, "'command.mock' in function 'my-func' (step 1.2 of 1) in block 'pre'", cmds[1].FullDisplayName())
		assert.Equal(t, "'command.mock' ('run-a-shell-thing') in function 'my-func' (step 1.3 of 1) in block 'pre'", cmds[2].FullDisplayName())
	})
}

func TestGetFullDisplayName(t *testing.T) {
	t.Run("NoCommandInfoProducesNoName", func(t *testing.T) {
		assert.Equal(t, "''", GetFullDisplayName("", "", BlockInfo{}, FunctionInfo{}))
	})
	t.Run("JustCommandName", func(t *testing.T) {
		assert.Equal(t, "'shell.exec'", GetFullDisplayName("shell.exec", "", BlockInfo{}, FunctionInfo{}))
	})
	t.Run("JustCommandNameAndDisplayName", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' ('run my script')", GetFullDisplayName("shell.exec", "run my script", BlockInfo{}, FunctionInfo{}))
	})
	t.Run("CommandNameAndBlockName", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' in block 'pre'", GetFullDisplayName("shell.exec", "", BlockInfo{
			Block: PreBlock,
		}, FunctionInfo{}))
	})
	t.Run("CommandNameAndBlockCommandNumber", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' (step 3 of 5)", GetFullDisplayName("shell.exec", "", BlockInfo{
			CmdNum:    3,
			TotalCmds: 5,
		}, FunctionInfo{}))
	})
	t.Run("CommandNameAndBlockNameAndBlockCommandNumber", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' (step 3 of 5) in block 'post'", GetFullDisplayName("shell.exec", "", BlockInfo{
			Block:     "post",
			CmdNum:    3,
			TotalCmds: 5,
		}, FunctionInfo{}))
	})
	t.Run("CommandNameAndBlockNameAndBlockCommandNumberWithOnlyOneCommandInBlock", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' (step 1 of 1) in block 'post'", GetFullDisplayName("shell.exec", "", BlockInfo{
			Block:     "post",
			CmdNum:    1,
			TotalCmds: 1,
		}, FunctionInfo{}))
	})
	t.Run("CommandNameAndFunc", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' in function 'my-func'", GetFullDisplayName("shell.exec", "", BlockInfo{}, FunctionInfo{
			Function: "my-func",
		}))
	})
	t.Run("CommandNameAndBlockCommandNumberAndFuncSubcommandNumber", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' in function 'my-func' (step 5.4 of 12)", GetFullDisplayName("shell.exec", "", BlockInfo{
			CmdNum:    5,
			TotalCmds: 12,
		}, FunctionInfo{
			Function:     "my-func",
			SubCmdNum:    4,
			TotalSubCmds: 10,
		}))
	})
	t.Run("CommandNameAndBlockCommandNumberAndFuncSubcommandNumberWithOnlyOneSubcommand", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' in function 'my-func' (step 5 of 12)", GetFullDisplayName("shell.exec", "", BlockInfo{
			CmdNum:    5,
			TotalCmds: 12,
		}, FunctionInfo{
			Function:     "my-func",
			SubCmdNum:    1,
			TotalSubCmds: 1,
		}))
	})
	t.Run("CommandNameAndFuncAndFuncSubcommandNumberButNoBlock", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' in function 'my-func'", GetFullDisplayName("shell.exec", "", BlockInfo{}, FunctionInfo{
			Function:     "my-func",
			SubCmdNum:    4,
			TotalSubCmds: 10,
		}))
	})
	t.Run("CommandNameAndDisplayNameAndAllBlockInfoAndAllFuncInfo", func(t *testing.T) {
		assert.Equal(t, "'shell.exec' ('run my script') in function 'my-func' (step 5.4 of 12) in block 'pre'", GetFullDisplayName("shell.exec", "run my script", BlockInfo{
			Block:     "pre",
			CmdNum:    5,
			TotalCmds: 12,
		}, FunctionInfo{
			Function:     "my-func",
			SubCmdNum:    4,
			TotalSubCmds: 10,
		}))
	})
}
