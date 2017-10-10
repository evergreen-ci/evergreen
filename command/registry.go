package command

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var evgRegistry *commandRegistry

func init() {
	evgRegistry = newCommandRegistry()

	cmds := map[string]CommandFactory{
		"archive.targz_pack":    tarballCreateFactory,
		"archive.targz_unpack":  tarballExtractFactory,
		"attach.results":        attachResultsFactory,
		"attach.xunit_results":  xunitResultsFactory,
		"expansions.fetch_vars": fetchVarsFactory,
		"expansions.update":     updateExpansionsFactory,
		"git.apply_patch":       gitApplyPatchFactory,
		"git.get_project":       gitFetchProjectFactory,
		"gotest.parse_files":    goTestFactory,
		"json.get":              taskDataGetFactory,
		"json.get_history":      taskDataHistoryFactory,
		"json.send":             taskDataSendFactory,
		"keyval.inc":            keyValIncFactory,
		"manifest.load":         manifestLoadFactory,
		"s3.get":                s3GetFactory,
		"s3.put":                s3PutFactory,
		"s3Copy.copy":           s3CopyFactory,
		"shell.cleanup":         shellCleanupFactory,
		"shell.exec":            shellExecFactory,
		"shell.track":           shellTrackFactory,
		"setup.initial":         initialSetupFactory,
	}

	for name, factory := range cmds {
		grip.EmergencyPanic(RegisterCommand(name, factory))
	}
}

func RegisterCommand(name string, factory CommandFactory) error {
	return errors.Wrap(evgRegistry.registerCommand(name, factory),
		"problem registering command")
}

func GetCommandFactory(name string) (CommandFactory, bool) {
	return evgRegistry.getCommandFactory(name)
}

func Render(c model.PluginCommandConf, fns map[string]*model.YAMLCommandSet) ([]Command, error) {
	return evgRegistry.renderCommands(c, fns)
}

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

func (r *commandRegistry) registerCommand(name string, factory CommandFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return errors.New("cannot register a command for the empty string ''")
	}

	if _, ok := r.cmds[name]; ok {
		return errors.Errorf("command '%s' is already registered", name)
	}

	if factory == nil {
		return errors.Errorf("cannot register a nil factory for command '%s'", name)
	}

	grip.Debug(message.Fields{
		"message": "registering command",
		"command": name,
	})

	r.cmds[name] = factory
	return nil
}

func (r *commandRegistry) getCommandFactory(name string) (CommandFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.cmds[name]
	return factory, ok
}

func (r *commandRegistry) renderCommands(cmd model.PluginCommandConf,
	funcs map[string]*model.YAMLCommandSet) ([]Command, error) {

	var (
		parsed []model.PluginCommandConf
		out    []Command
		errs   []string
		err    error
	)

	if name := cmd.Function; name != "" {
		cmds, ok := funcs[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("function '%s' not found in project functions", name))
		} else {
			for _, c := range cmds.List() {
				if c.Function != "" {
					errs = append(errs, fmt.Sprintf("can not reference a function within a "+
						"function: '%s' referenced within '%s'", c.Function, name))
					continue
				}

				// if no command specific type, use the function's command type
				if c.Type == "" {
					c.Type = cmd.Type
				}

				if c.DisplayName == "" {
					c.DisplayName = fmt.Sprintf(`'%v' in "%v"`, c.Command, name)
				}

				parsed = append(parsed, c)
			}
		}
	} else {
		parsed = append(parsed, cmd)
	}

	for _, c := range parsed {
		factory, ok := r.getCommandFactory(c.Command)
		if !ok {
			errs = append(errs, fmt.Sprintf("command '%s' is not registered", c.Command))
			continue
		}

		command := factory()
		if err = command.ParseParams(c.Params); err != nil {
			errs = append(errs, "problem parsing input of %s (%s)", c.Command, c.DisplayName)
			continue
		}
		command.SetType(c.Type)
		command.SetDisplayName(c.DisplayName)
		command.SetIdleTimeout(time.Duration(c.TimeoutSecs) * time.Second)
		if cmd.TimeoutSecs > 0 {
			command.SetIdleTimeout(time.Duration(cmd.TimeoutSecs) * time.Second)
		}

		out = append(out, command)
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	return out, nil
}
