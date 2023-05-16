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

func Render(c model.PluginCommandConf, project *model.Project, block string) ([]Command, error) {
	return evgRegistry.renderCommands(c, project, block)
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
	project *model.Project, block string) ([]Command, error) {

	var (
		parsed []model.PluginCommandConf
		out    []Command
	)
	catcher := grip.NewBasicCatcher()

	if block != "" {
		block = fmt.Sprintf(`in "%v"`, block)
	}

	if name := commandInfo.Function; name != "" {
		cmds, ok := project.Functions[name]
		if !ok {
			catcher.Errorf("function '%s' not found in project functions", name)
		} else if cmds != nil {
			for i, c := range cmds.List() {
				if c.Function != "" {
					catcher.Errorf("cannot reference a function ('%s') within another function ('%s')", c.Function, name)
					continue
				}

				// if no command specific type, use the function's command type
				if c.Type == "" {
					c.Type = commandInfo.Type
				}

				if c.DisplayName == "" {
					c.DisplayName = fmt.Sprintf(`'%v' in "%v" %s (#%d)`, c.Command, name, block, i+1)
				}

				if c.TimeoutSecs == 0 {
					c.TimeoutSecs = commandInfo.TimeoutSecs
				}

				parsed = append(parsed, c)
			}
		}
	} else {
		if commandInfo.DisplayName == "" {
			commandInfo.DisplayName = fmt.Sprintf(`'%v' %s `, commandInfo.Command, block)
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
