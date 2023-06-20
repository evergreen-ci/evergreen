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
// kim: TODO: test display name defaulting.
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

	var (
		parsed []model.PluginCommandConf
		out    []Command
	)
	catcher := grip.NewBasicCatcher()

	if funcName := commandInfo.Function; funcName != "" {
		cmds, ok := project.Functions[funcName]
		if !ok {
			catcher.Errorf("function '%s' not found in project functions", funcName)
		} else if cmds != nil {
			for i, c := range cmds.List() {
				if c.Function != "" {
					catcher.Errorf("cannot reference a function ('%s') within another function ('%s')", c.Function, funcName)
					continue
				}

				// if no command specific type, use the function's command type
				if c.Type == "" {
					c.Type = commandInfo.Type
				}

				if c.DisplayName == "" {
					// kim: TODO: test that UI display name displays the name
					// more like how it's displayed in the logs.
					// kim: TODO: may be better to instead display
					// (#STEP_NUMBER) as "step FUNC.SUBCOMMAND of
					// TOTAL_IN_BLOCK" format, would have to pass in total
					// number of commands in the block.
					funcInfo := FunctionInfo{
						Function:  c.Function,
						SubCmdNum: i + 1,
					}
					c.DisplayName = GetDefaultDisplayName(c.Command, blockInfo, funcInfo)
				}

				if c.TimeoutSecs == 0 {
					c.TimeoutSecs = commandInfo.TimeoutSecs
				}

				parsed = append(parsed, c)
			}
		}
	} else {
		if commandInfo.DisplayName == "" {
			// kim: TODO: standardize this display name against function.
			// kim: TODO: test
			commandInfo.DisplayName = GetDefaultDisplayName(commandInfo.Command, blockInfo, FunctionInfo{})
		}
		parsed = append(parsed, commandInfo)
	}

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
			catcher.Wrapf(err, "parsing parameters for command '%s' ('%s')", c.Command, c.DisplayName)
			continue
		}
		cmd.SetType(c.GetType(project))
		cmd.SetDisplayName(c.DisplayName)
		cmd.SetIdleTimeout(time.Duration(c.TimeoutSecs) * time.Second)

		out = append(out, cmd)
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return out, nil
}

// BlockInfo contains information about the enclosing block in which a function
// or standalone command runs. For example, this would contain information about
// the pre block that contains a particular shell.exec command.
type BlockInfo struct {
	// Block is the name of the block that the command is part of.
	Block string
	// CmdNum is the ordinal of a command in the block.
	CmdNum int
	// TotalCmds is the total number of commands in the block.
	TotalCmds int
}

// FunctionInfo contains information about the enclosing block in which a
// command listed within a function runs. For example, this would contain
// information about the second shell.exec that runs in a function.
type FunctionInfo struct {
	// Function is the name of the function that the command is part of.
	Function string
	// SubCmdNum is the ordinal of the command within the function.
	SubCmdNum int
}

// GetDefaultDisplayName returns the default display name for a command.
// cmdNum is command/function number, subCmdNum only applies for subcmds in
// functions
// kim: TODO: add unit tests
// kim: TODO: potentially pass in BlockInfo and FuncInfo structs for
// cleanliness.
func GetDefaultDisplayName(commandName string, blockInfo BlockInfo, funcInfo FunctionInfo) string {
	displayName := fmt.Sprintf("'%s'", commandName)
	if funcInfo.Function != "" {
		displayName = fmt.Sprintf("%s in function '%s'", displayName, funcInfo.Function)
	}
	if blockInfo.CmdNum > 0 && blockInfo.TotalCmds > 0 {
		if funcInfo.SubCmdNum > 0 {
			displayName = fmt.Sprintf("%s (step %d.%d of %d)", displayName, blockInfo.CmdNum, funcInfo.SubCmdNum, blockInfo.TotalCmds)
		} else {
			displayName = fmt.Sprintf("%s (step %d of %d)", displayName, blockInfo.CmdNum, blockInfo.TotalCmds)
		}
	}
	// kim: TODO: decide if having the block is worth it or not. It makes the
	// name pretty long, and is omitted when it's running the main task block,
	// so it's already a little wonky.
	// Alternatively, maybe pass a showBlock option to DisplayName to choose
	// when/where to show the block name.
	if blockInfo.Block != "" {
		displayName = fmt.Sprintf("%s in block '%s'", displayName, blockInfo.Block)
	}
	return displayName
}
