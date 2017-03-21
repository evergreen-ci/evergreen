package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/gorilla/context"
	"github.com/mongodb/grip/slogger"
)

var (
	// These are slices of all plugins that have made themselves
	// visible to the Evergreen system. A Plugin can add itself by appending an instance
	// of itself to these slices on init, i.e. by adding the following to its
	// source file:
	//  func init(){
	//  	plugin.Publish(&MyCoolPlugin{})
	//  }
	// This list is then used by Agent, API, and UI Server code to register
	// the published plugins.
	CommandPlugins []CommandPlugin
	APIPlugins     []APIPlugin
	UIPlugins      []UIPlugin
)

// Registry manages available plugins, and produces instances of
// Commands from model.PluginCommandConf, a command's representation in the config.
type Registry interface {
	// Make the given plugin available for usage with tasks.
	// Returns an error if the plugin is invalid, or conflicts with an already
	// registered plugin.
	Register(p CommandPlugin) error

	// Parse the parameters in the given command and return a corresponding
	// Command. Returns ErrUnknownPlugin if the command refers to
	// a plugin that isn't registered, or some other error if the plugin
	// can't parse valid parameters from the command.
	GetCommands(command model.PluginCommandConf,
		funcs map[string]*model.YAMLCommandSet) ([]Command, error)

	// ParseCommandConf takes a plugin command and either returns a list of
	// command(s) defined by the function (if the plugin command is a function),
	// or a list containing the command itself otherwise.
	ParseCommandConf(command model.PluginCommandConf,
		funcs map[string]*model.YAMLCommandSet) ([]model.PluginCommandConf, error)
}

// Logger allows any plugin to log to the appropriate place with any
// The agent (which provides each plugin execution with a Logger implementation)
// handles sending log data to the remote server
type Logger interface {
	// Log a message locally. Will be persisted in the log file on the builder, but
	// not appended to the log data sent to API server.
	LogLocal(level slogger.Level, messageFmt string, args ...interface{})

	// Log data about the plugin's execution.
	LogExecution(level slogger.Level, messageFmt string, args ...interface{})

	// Log data from the plugin's actual commands, e.g. shell script output or
	// stdout/stderr messages from a command
	LogTask(level slogger.Level, messageFmt string, args ...interface{})

	// Log system-level information (resource usage, ), etc.
	LogSystem(level slogger.Level, messageFmt string, args ...interface{})

	// Returns the task log writer as an io.Writer, for use in io-related
	// functions, e.g. io.Copy
	GetTaskLogWriter(level slogger.Level) io.Writer

	// Returns the system log writer as an io.Writer, for use in io-related
	// functions, e.g. io.Copy
	GetSystemLogWriter(level slogger.Level) io.Writer

	// Trigger immediate flushing of any buffered log messages.
	Flush()
}

// PluginCommunicator is an interface that allows a plugin's client-side processing
// (running inside an agent) to communicate with the routes it has installed
// on the server-side via HTTP GET and POST requests.
// Does not handle retrying of requests. The caller must also ensure that
// the Body of the returned http responses are closed.
type PluginCommunicator interface {

	// Make a POST request to the given endpoint by submitting 'data' as
	// the request body, marshaled as JSON.
	TaskPostJSON(endpoint string, data interface{}) (*http.Response, error)

	// Make a GET request to the given endpoint with content type "application/json"
	TaskGetJSON(endpoint string) (*http.Response, error)

	// Make a POST request against the results api endpoint
	TaskPostResults(results *task.TestResults) error

	// Make a POST request against the test_log api endpoint
	TaskPostTestLog(log *model.TestLog) (string, error)

	// Make a POST request against the files api endpoint
	PostTaskFiles(files []*artifact.File) error
}

// Plugin defines the interface that all evergreen plugins must implement in order
// to register themselves with Evergreen. A plugin must also implement one of the
// PluginCommand, APIPlugin, or UIPlugin interfaces in order to do useful work.
type Plugin interface {
	// Returns the name to identify this plugin when registered.
	Name() string
}

// CommandPlugin is implemented by plugins that add new task commands
// that are run by the agent.
type CommandPlugin interface {
	Plugin

	// Returns an ErrUnknownCommand if no such command exists
	NewCommand(commandName string) (Command, error)
}

// APIPlugin is implemented by plugins that need to add new API hooks for
// new task commands.
// TODO: should this also require PluginCommand be implemented?
type APIPlugin interface {
	Plugin

	// Configure reads in a settings map from the Evergreen config file.
	Configure(conf map[string]interface{}) error

	// Install any server-side handlers needed by this plugin in the API server
	GetAPIHandler() http.Handler
}

type UIPlugin interface {
	Plugin

	// Install any server-side handlers needed by this plugin in the UI server
	GetUIHandler() http.Handler

	// GetPanelConfig returns a pointer to a plugin's UI configuration.
	// or an error, if an error occur while trying to generate the config
	// A nil pointer represents a plugin without a UI presence, and is
	// not an error.
	GetPanelConfig() (*PanelConfig, error)

	// Configure reads in a settings map from the Evergreen config file.
	Configure(conf map[string]interface{}) error
}

// AppUIPlugin represents a UIPlugin that also has a page route.
type AppUIPlugin interface {
	UIPlugin

	// GetAppPluginInfo returns all the information
	// needed for the UI server to render a page from the navigation bar.
	GetAppPluginInfo() *UIPage
}

// Publish is called in a plugin's "init" func to
// announce that plugin's presence to the entire plugin package.
// This architecture is designed to make it easy to add
// new external plugin code to Evergreen by simply importing the
// new plugin's package in plugin/config/installed_plugins.go
//
// Packages implementing the Plugin interface MUST call Publish in their
// init code in order for Evergreen to detect and use them. A plugin must
// also implement one of CommandPlugin, APIPlugin, or UIPlugin in order to
// be useable.
//
// See the documentation of the 10gen.com/mci/plugin/config package for more
func Publish(plugin Plugin) {
	published := false
	if asCommand, ok := plugin.(CommandPlugin); ok {
		CommandPlugins = append(CommandPlugins, asCommand)
		published = true
	}
	if asAPI, ok := plugin.(APIPlugin); ok {
		APIPlugins = append(APIPlugins, asAPI)
		published = true
	}
	if asUI, ok := plugin.(UIPlugin); ok {
		UIPlugins = append(UIPlugins, asUI)
		published = true
	}
	if !published {
		panic(fmt.Sprintf("Plugin '%v' does not implement any of CommandPlugin, APIPlugin, or UIPlugin"))
	}
}

// ErrUnknownPlugin indicates a plugin was requested that is not registered in the plugin manager.
type ErrUnknownPlugin struct {
	PluginName string
}

// Error returns information about the non-registered plugin;
// satisfies the error interface
func (eup *ErrUnknownPlugin) Error() string {
	return fmt.Sprintf("Unknown plugin: '%v'", eup.PluginName)
}

// ErrUnknownCommand indicates a command is referenced from a plugin that does not support it.
type ErrUnknownCommand struct {
	CommandName string
}

func (eup *ErrUnknownCommand) Error() string {
	return fmt.Sprintf("Unknown command: '%v'", eup.CommandName)
}

// WriteJSON writes data encoded in JSON format (Content-type: "application/json")
// to the ResponseWriter with the supplied HTTP status code. Writes a 500 error
// if the data cannot be JSON-encoded.
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(out)
}

type pluginTaskContext int

const pluginTaskContextKey pluginTaskContext = 0

// SetTask puts the task for an API request into the context of a request.
// This task can be retrieved in a handler function by using "GetTask()"
func SetTask(request *http.Request, task *task.Task) {
	context.Set(request, pluginTaskContextKey, task)
}

// GetTask returns the task object for a plugin API request at runtime,
// it is a valuable helper function for API PluginRoute handlers.
func GetTask(request *http.Request) *task.Task {
	if rv := context.Get(request, pluginTaskContextKey); rv != nil {
		return rv.(*task.Task)
	}
	return nil
}

// SimpleRegistry is a simple, local, map-based implementation
// of a plugin registry.
type SimpleRegistry struct {
	pluginsMapping map[string]CommandPlugin
}

// NewSimpleRegistry returns an initialized SimpleRegistry
func NewSimpleRegistry() *SimpleRegistry {
	registry := &SimpleRegistry{
		pluginsMapping: map[string]CommandPlugin{},
	}
	return registry
}

// Register makes a given plugin and its commands available to the agent code.
// This function returns an error if a plugin of the same name is already registered.
func (sr *SimpleRegistry) Register(p CommandPlugin) error {
	if _, hasKey := sr.pluginsMapping[p.Name()]; hasKey {
		return fmt.Errorf("Plugin with name '%v' has already been registered", p.Name())
	}
	sr.pluginsMapping[p.Name()] = p
	return nil
}

func (sr *SimpleRegistry) ParseCommandConf(cmd model.PluginCommandConf, funcs map[string]*model.YAMLCommandSet) ([]model.PluginCommandConf, error) {

	if funcName := cmd.Function; funcName != "" {
		cmds, ok := funcs[funcName]
		if !ok {
			return nil, fmt.Errorf("function '%v' not found in project functions", funcName)
		}

		cmdList := cmds.List()

		cmdsParsed := make([]model.PluginCommandConf, 0, len(cmdList))

		for _, c := range cmdList {
			if c.Function != "" {
				return nil, fmt.Errorf("can not reference a function within "+
					"a function: '%v' referenced within '%v'", c.Function, funcName)
			}

			// if no command specific type, use the function's command type
			if c.Type == "" {
				c.Type = cmd.Type
			}

			// use function name if no command display name exists
			if c.DisplayName == "" {
				c.DisplayName = fmt.Sprintf(`'%v' in "%v"`, c.Command, funcName)
			}

			cmdsParsed = append(cmdsParsed, c)
		}

		return cmdsParsed, nil
	}

	return []model.PluginCommandConf{cmd}, nil
}

// GetCommands finds a registered plugin for the given plugin command config
// Returns ErrUnknownPlugin if the cmd refers to a plugin that isn't registered,
// or some other error if the plugin can't parse valid parameters from the conf.
func (sr *SimpleRegistry) GetCommands(cmd model.PluginCommandConf, funcs map[string]*model.YAMLCommandSet) ([]Command, error) {

	cmds, err := sr.ParseCommandConf(cmd, funcs)
	if err != nil {
		return nil, err
	}

	cmdsParsed := make([]Command, 0, len(cmds))

	for _, c := range cmds {
		pluginNameParts := strings.Split(c.Command, ".")
		if len(pluginNameParts) != 2 {
			return nil, fmt.Errorf("Value of 'command' should be formatted: 'plugin_name.command_name'")
		}
		plugin, hasKey := sr.pluginsMapping[pluginNameParts[0]]
		if !hasKey {
			return nil, &ErrUnknownPlugin{pluginNameParts[0]}
		}

		command, err := plugin.NewCommand(pluginNameParts[1])
		if err != nil {
			return nil, err
		}

		if err = command.ParseParams(c.Params); err != nil {
			return nil, err
		}
		cmdsParsed = append(cmdsParsed, command)
	}
	return cmdsParsed, nil
}

// Command is an interface that defines a command for a plugin.
// A Command takes parameters as a map, and is executed after
// those parameters are parsed.
type Command interface {
	// ParseParams takes a map of fields to values extracted from
	// the project config and passes them to the command. Any
	// errors parsing the information are returned.
	ParseParams(params map[string]interface{}) error

	// Execute runs the command using the agent's logger, communicator,
	// task config, and a channel for interrupting long-running commands.
	// Execute is called after ParseParams.
	Execute(logger Logger, pluginCom PluginCommunicator,
		conf *model.TaskConfig, stopChan chan bool) error

	// A string name for the command
	Name() string

	// Plugin name
	Plugin() string
}
