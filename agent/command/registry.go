package command

import (
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var evgRegistry *commandRegistry

func init() {
	evgRegistry = newCommandRegistry()

	cmds := map[string]CommandFactory{
		"archive.targz_pack":                    tarballCreateFactory,
		"archive.targz_extract":                 tarballExtractFactory,
		"archive.zip_pack":                      zipArchiveCreateFactory,
		"archive.zip_extract":                   zipExtractFactory,
		"archive.auto_extract":                  autoExtractFactory,
		evergreen.AttachResultsCommandName:      attachResultsFactory,
		evergreen.AttachXUnitResultsCommandName: xunitResultsFactory,
		evergreen.AttachArtifactsCommandName:    attachArtifactsFactory,
		evergreen.HostCreateCommandName:         createHostFactory,
		"ec2.assume_role":                       ec2AssumeRoleFactory,
		"host.list":                             listHostFactory,
		"expansions.update":                     updateExpansionsFactory,
		"expansions.write":                      writeExpansionsFactory,
		"generate.tasks":                        generateTaskFactory,
		"git.apply_patch":                       gitApplyPatchFactory,
		"git.get_project":                       gitFetchProjectFactory,
		"git.merge_pr":                          gitMergePRFactory,
		"git.push":                              gitPushFactory,
		"gotest.parse_files":                    goTestFactory,
		"keyval.inc":                            keyValIncFactory,
		"mac.sign":                              macSignFactory,
		"manifest.load":                         manifestLoadFactory,
		"perf.send":                             perfSendFactory,
		"downstream_expansions.set":             setExpansionsFactory,
		"s3.get":                                s3GetFactory,
		"s3.put":                                s3PutFactory,
		"s3Copy.copy":                           s3CopyFactory,
		evergreen.S3PushCommandName:             s3PushFactory,
		evergreen.S3PullCommandName:             s3PullFactory,
		evergreen.ShellExecCommandName:          shellExecFactory,
		"subprocess.exec":                       subprocessExecFactory,
		"setup.initial":                         initialSetupFactory,
		"timeout.update":                        timeoutUpdateFactory,
	}

	for name, factory := range cmds {
		grip.EmergencyPanic(RegisterCommand(name, factory))
	}
}

func RegisterCommand(name string, factory CommandFactory) error {
	return errors.Wrapf(evgRegistry.registerCommand(name, factory), "registering command '%s'", name)
}

func GetCommandFactory(name string) (CommandFactory, bool) {
	return evgRegistry.getCommandFactory(name)
}

// Render takes a command specification and returns the commands to actually
// run. It resolves the command specification into either a single command (in
// the case of standalone command) or a list of commands (in the case of a
// function).
func Render(c model.PluginCommandConf, project *model.Project, blockInfo BlockInfo) ([]Command, error) {
	return evgRegistry.renderCommands(c, project, blockInfo)
}

func RegisteredCommandNames() []string { return evgRegistry.registeredCommandNames() }

type CommandFactory func() Command

type commandRegistry struct {
	mu   *sync.RWMutex
	cmds map[string]CommandFactory
}

func newCommandRegistry() *commandRegistry {
	return &commandRegistry{
		cmds: map[string]CommandFactory{},
		mu:   &sync.RWMutex{},
	}
}

func (r *commandRegistry) registeredCommandNames() []string {
	out := []string{}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for name := range r.cmds {
		out = append(out, name)
	}

	return out
}

func (r *commandRegistry) registerCommand(name string, factory CommandFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return errors.New("cannot register a command without a name")
	}

	if _, ok := r.cmds[name]; ok {
		return errors.Errorf("command '%s' is already registered", name)
	}

	if factory == nil {
		return errors.Errorf("cannot register a nil factory for command '%s'", name)
	}

	r.cmds[name] = factory
	return nil
}

func (r *commandRegistry) getCommandFactory(name string) (CommandFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.cmds[name]
	return factory, ok
}

func (r *commandRegistry) renderCommands(commandInfo model.PluginCommandConf,
	project *model.Project, blockInfo BlockInfo) ([]Command, error) {

	var parsed []model.PluginCommandConf

	catcher := grip.NewBasicCatcher()

	if funcName := commandInfo.Function; funcName != "" {
		cmds, ok := project.Functions[funcName]
		if !ok {
			catcher.Errorf("function '%s' not found in project functions", funcName)
		} else if cmds != nil {
			cmdsInFunc := cmds.List()
			for i, c := range cmdsInFunc {
				if c.Function != "" {
					catcher.Errorf("cannot reference a function ('%s') within another function ('%s')", c.Function, funcName)
					continue
				}

				// If there's no command-specific type/timeout, use the
				// function's command type/timeout
				if c.Type == "" {
					c.Type = commandInfo.Type
				}
				if c.TimeoutSecs == 0 {
					c.TimeoutSecs = commandInfo.TimeoutSecs
				}

				funcInfo := FunctionInfo{
					Function:     funcName,
					SubCmdNum:    i + 1,
					TotalSubCmds: len(cmdsInFunc),
				}
				c.DisplayName = GetFullDisplayName(c.Command, c.DisplayName, blockInfo, funcInfo)

				parsed = append(parsed, c)
			}
		}
	} else {
		commandInfo.DisplayName = GetFullDisplayName(commandInfo.Command, commandInfo.DisplayName, blockInfo, FunctionInfo{})
		parsed = append(parsed, commandInfo)
	}

	var out []Command
	for _, c := range parsed {
		factory, ok := r.getCommandFactory(c.Command)
		if !ok {
			catcher.Errorf("command '%s' is not registered", c.Command)
			continue
		}

		cmd := factory()
		// Note: this parses the parameters before expansions are applied.
		// Expansions are only available when the command is executed.
		if err := cmd.ParseParams(c.Params); err != nil {
			catcher.Wrapf(err, "parsing parameters for command %s", c.DisplayName)
			continue
		}
		cmd.SetType(c.GetType(project))
		cmd.SetFullDisplayName(c.DisplayName)
		cmd.SetIdleTimeout(time.Duration(c.TimeoutSecs) * time.Second)

		out = append(out, cmd)
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return out, nil
}

// BlockType is the name of the block that a command runs in.
type BlockType string

const (
	MainTaskBlock      BlockType = ""
	TaskTimeoutBlock   BlockType = "timeout"
	PreBlock           BlockType = "pre"
	SetupTaskBlock     BlockType = "setup_task"
	TeardownTaskBlock  BlockType = "teardown_task"
	SetupGroupBlock    BlockType = "setup_group"
	TeardownGroupBlock BlockType = "teardown_group"
	PostBlock          BlockType = "post"
	TaskSyncBlock      BlockType = "task_sync"
)

// BlockInfo contains information about the enclosing block in which a function
// or standalone command runs. For example, this would contain information about
// the pre block that contains a particular shell.exec command.
type BlockInfo struct {
	// Block is the name of the block that the command is part of.
	Block BlockType
	// CmdNum is the ordinal of a command in the block.
	CmdNum int
	// TotalCmds is the total number of commands in the block.
	TotalCmds int
}

// FunctionInfo contains information about the enclosing function in which a
// command runs. For example, this would contain information about the second
// shell.exec that runs in a function.
type FunctionInfo struct {
	// Function is the name of the function that the command is part of.
	Function string
	// SubCmdNum is the ordinal of the command within the function.
	SubCmdNum int
	// TotalSubCmds is the total number of sub-commands within the function.
	TotalSubCmds int
}

// GetFullDisplayName returns the full, unambiguous display name for a command.
// cmdName is the type of command (e.g. shell.exec), displayName is the
// human-readable display name (if specified), or the command name (if no
// display name is given). blockInfo and funcInfo include contextual information
// about the block/func that the command is running in.
func GetFullDisplayName(cmdName, displayName string, blockInfo BlockInfo, funcInfo FunctionInfo) string {
	fullName := fmt.Sprintf("'%s'", cmdName)
	if displayName != "" {
		fullName = fmt.Sprintf("%s ('%s')", fullName, displayName)
	}
	if funcInfo.Function != "" {
		fullName = fmt.Sprintf("%s in function '%s'", fullName, funcInfo.Function)
	}
	if blockInfo.CmdNum > 0 && blockInfo.TotalCmds > 0 {
		if funcInfo.SubCmdNum > 0 && funcInfo.TotalSubCmds > 1 {
			// Include the function sub-command number only if the function runs
			// more than one command.
			fullName = fmt.Sprintf("%s (step %d.%d of %d)", fullName, blockInfo.CmdNum, funcInfo.SubCmdNum, blockInfo.TotalCmds)
		} else {
			fullName = fmt.Sprintf("%s (step %d of %d)", fullName, blockInfo.CmdNum, blockInfo.TotalCmds)
		}
	}
	if blockInfo.Block != "" {
		fullName = fmt.Sprintf("%s in block '%s'", fullName, blockInfo.Block)
	}
	return fullName
}
