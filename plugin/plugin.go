package plugin

import (
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/gorilla/context"
	"io"
	"net/http"
	"strings"
)

// Published is an array of all plugins that have made themselves
// visible to the Evergreen system. A Plugin can add itself by appending an instance
// of itself to this array on init, i.e. by adding the following to its
// source file:
//  func init(){
//  	plugin.Publish(&MyCoolPlugin{})
//  }
// This list is then used by Agent, API, and UI Server code to register
// the published plugins.
var Published []Plugin

// Publish is called in a plugin's "init" func to
// announce that plugin's presence to the entire plugin package.
// This architecture is designed to make it easy to add
// new external plugin code to Evergreen by simply importing the
// new plugin's package in plugin/config/installed_plugins.go
//
// Packages implementing the Plugin interface MUST call Publish in their
// init code in order for Evergreen to detect and use them.
//
// See the documentation of the 10gen.com/mci/plugin/config package for more
func Publish(plugin Plugin) {
	Published = append(Published, plugin)
}

// ErrUnknownPlugin indicates a plugin was requested that is not registered in the plugin manager.
type ErrUnknownPlugin struct {
	PluginName string
}

// Error returns information about the non-registered plugin;
// satisfies the error interface
func (self *ErrUnknownPlugin) Error() string {
	return fmt.Sprintf("Unknown plugin: '%v'", self.PluginName)
}

// ErrUnknownCommand indicates a command is referenced from a plugin that does not support it.
type ErrUnknownCommand struct {
	CommandName string
}

func (self *ErrUnknownCommand) Error() string {
	return fmt.Sprintf("Unknown command: '%v'", self.CommandName)
}

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

//SetTask puts the task for an API request into the context of a request.
//This task can be retrieved in a handler function by using "GetTask()"
func SetTask(request *http.Request, task *model.Task) {
	context.Set(request, pluginTaskContextKey, task)
}

//GetTask returns the task object for a plugin API request at runtime,
//it is a valuable helper function for API PluginRoute handlers.
func GetTask(request *http.Request) *model.Task {
	if rv := context.Get(request, pluginTaskContextKey); rv != nil {
		return rv.(*model.Task)
	}
	return nil
}

// Registry manages available plugins, and produces instances of
// Commands from model.PluginCommandConf, a command's representation in the config.
type Registry interface {
	//Make the given plugin available for usage with tasks.
	//Returns an error if the plugin is invalid, or conflicts with an already
	//registered plugin.
	Register(p Plugin) error

	//Parse the params in the given command and return a corresponding
	//Command. Returns ErrUnknownPlugin if the command refers to
	//a plugin that isn't registered, or some other error if the plugin
	//can't parse valid parameters from the command.
	GetCommands(command model.PluginCommandConf,
		funcs map[string]*model.YAMLCommandSet) ([]Command, error)
}

//Logger allows any plugin to log to the appropriate place with any
//The agent (which provides each plugin execution with a Logger implementation)
//handles sending log data to the remote server
type Logger interface {
	//Log a message locally. Will be persisted in the log file on the builder, but
	//not appended to the log data sent to API server.
	LogLocal(level slogger.Level, messageFmt string, args ...interface{})

	//Log data about the plugin's execution.
	LogExecution(level slogger.Level, messageFmt string, args ...interface{})

	//Log data from the plugin's actual commands, e.g. shell script output or
	//stdout/stderr messages from a command
	LogTask(level slogger.Level, messageFmt string, args ...interface{})

	//Log system-level information (resource usage, ), etc.
	LogSystem(level slogger.Level, messageFmt string, args ...interface{})

	//Returns the log writer as an io.Writer, for use in io-related
	//functions, e.g. io.Copy
	GetTaskLogWriter(level slogger.Level) io.Writer

	//Trigger immediate flushing of any buffered log messages.
	Flush()
}

// PluginCommunicator is an interface that allows a plugin's client-side processing
// (running inside an agent) to communicate with the routes it has installed
// on the server-side via HTTP GET and POST requests.
// Does not handle retrying of requests. The caller must also ensure that
// the Body of the returned http responses are closed.
type PluginCommunicator interface {

	//Make a POST request to the given endpoint by submitting 'data' as
	//the request body, marshalled as JSON.
	TaskPostJSON(endpoint string, data interface{}) (*http.Response, error)

	//Make a GET request to the given endpoint with content type "application/json"
	TaskGetJSON(endpoint string) (*http.Response, error)

	//Make a POST request against the results api endpoint
	TaskPostResults(results *model.TestResults) error

	//Make a POST request against the test_log api endpoint
	TaskPostTestLog(log *model.TestLog) (string, error)

	//Make a POST request against the files api endpoint
	PostTaskFiles(files []*artifact.File) error
}

//Plugin defines the interface that all evergreen plugins must implement in order
//to register themselves with the agent and API/UI servers.
type Plugin interface {
	//Returns the name to identify this plugin when registered.
	Name() string

	Configure(conf map[string]interface{}) error

	// Install any server-side handlers needed by this plugin in the API server
	GetAPIHandler() http.Handler

	// Install any server-side handlers needed by this plugin in the UI server
	GetUIHandler() http.Handler

	//GetPanelConfig returns a pointer to a plugin's UI configuration.
	//or an error, if an error occur while trying to generate the config
	//A nil pointer represents a plugin without a UI presence, and is
	//not an error.
	GetPanelConfig() (*PanelConfig, error)

	//Returns an ErrUnknownCommand if no such command exists
	NewCommand(commandName string) (Command, error)
}

//SimpleRegistry is a simple, local, map-based implementation
//of a plugin registry.
type SimpleRegistry struct {
	pluginsMapping map[string]Plugin
}

//NewSimpleRegistry returns an initialized SimpleRegistry
func NewSimpleRegistry() *SimpleRegistry {
	registry := &SimpleRegistry{
		pluginsMapping: map[string]Plugin{},
	}
	return registry
}

//Register makes a given plugin and its commands available to the agent code.
//This function returns an error if a plugin of the same name is already
//registered.
func (self *SimpleRegistry) Register(p Plugin) error {
	if _, hasKey := self.pluginsMapping[p.Name()]; hasKey {
		return fmt.Errorf("Plugin with name '%v' has already been registered", p.Name())
	}
	self.pluginsMapping[p.Name()] = p
	return nil
}

//GetCommands finds a registered plugin for the given plugin command config
//Returns ErrUnknownPlugin if the cmd refers to a plugin that isn't registered,
//or some other error if the plugin can't parse valid parameters from the conf.
func (self *SimpleRegistry) GetCommands(cmd model.PluginCommandConf, funcs map[string]*model.YAMLCommandSet) ([]Command, error) {

	var cmds []model.PluginCommandConf
	if cmd.Function != "" {
		var hasKey bool
		funcName := cmd.Function
		cmdSet, hasKey := funcs[funcName]
		if !hasKey {
			return nil, fmt.Errorf("function '%v' not found in project functions", funcName)
		}
		cmds = cmdSet.List()

		for _, c := range cmds {
			if c.Function != "" {
				return nil, fmt.Errorf("can not reference a function within "+
					"a function: '%v' referenced within '%v'", cmd.Function, funcName)
			}
		}
	} else {
		cmds = []model.PluginCommandConf{cmd}
	}

	cmdsParsed := make([]Command, 0, len(cmds))

	for _, c := range cmds {
		pluginNameParts := strings.Split(c.Command, ".")
		if len(pluginNameParts) != 2 {
			return nil, fmt.Errorf("Value of 'command' should be formatted: 'plugin_name.command_name'")
		}
		plugin, hasKey := self.pluginsMapping[pluginNameParts[0]]
		if !hasKey {
			return nil, &ErrUnknownPlugin{pluginNameParts[0]}
		}
		command, err := plugin.NewCommand(pluginNameParts[1])
		if err != nil {
			return nil, err
		}

		err = command.ParseParams(c.Params)
		if err != nil {
			return nil, err
		}
		cmdsParsed = append(cmdsParsed, command)
	}
	return cmdsParsed, nil
}

//Command is an interface that defines a command for a plugin.
//A Command takes parameters as a map, and is executed after
//those parameters are parsed.
type Command interface {
	//ParseParams takes a map of fields to values extracted from
	//the project config and passes them to the command. Any
	//errors parsing the information are returned.
	ParseParams(params map[string]interface{}) error

	//Execute runs the command using the agent's logger, communicator,
	//task config, and a channel for interrupting long-running commands.
	//Execute is called after ParseParams.
	Execute(logger Logger, pluginCom PluginCommunicator,
		conf *model.TaskConfig, stopChan chan bool) error

	//A string name for the command
	Name() string

	//Plugin name
	Plugin() string
}
