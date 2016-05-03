package expansions

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
)

func init() {
	plugin.Publish(&ExpansionsPlugin{})
}

const (
	ExpansionsPluginName = "expansions"
	UpdateVarsCmdName    = "update"
)

// ExpansionsPlugin handles updating expansions in a task at runtime.
type ExpansionsPlugin struct{}

// Name fulfills the Plugin interface.
func (self *ExpansionsPlugin) Name() string {
	return ExpansionsPluginName
}

// GetRoutes registers the API handler for fetching expansion variables
// from the API server.
func (self *ExpansionsPlugin) GetAPIHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc(fmt.Sprintf("/%v", FetchVarsRoute), FetchVarsHandler) // GET
	r.HandleFunc("/", http.NotFound)
	return r
}

func (self *ExpansionsPlugin) Configure(map[string]interface{}) error {
	return nil
}

// NewCommand fulfills the Plugin interface.
func (self *ExpansionsPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == UpdateVarsCmdName {
		return &UpdateCommand{}, nil
	} else if cmdName == FetchVarsCmdname {
		return &FetchVarsCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}

// UpdateCommand reads in a set of new expansions and updates the
// task's expansions at runtime. UpdateCommand can take a list
// of update expansion pairs and/or a file of expansion pairs
type UpdateCommand struct {
	// Key-value pairs for updating the task's parameters with
	Updates []PutCommandParams `mapstructure:"updates"`

	// Filename for a yaml file containing expansion updates
	// in the form of
	//   "expansion_key: expansions_value"
	YamlFile string `mapstructure:"file"`
}

// PutCommandParams are pairings of expansion names
// and the value they expand to
type PutCommandParams struct {
	// The name of the expansion
	Key string

	// The expanded value
	Value string

	// Can optionally concat a string to the end of the current value
	Concat string
}

func (self *UpdateCommand) Name() string {
	return UpdateVarsCmdName
}

func (self *UpdateCommand) Plugin() string {
	return ExpansionsPluginName
}

// ParseParams validates the input to the UpdateCommand, returning and error
// if something is incorrect. Fulfills Command interface.
func (self *UpdateCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, self)
	if err != nil {
		return err
	}

	for _, item := range self.Updates {
		if item.Key == "" {
			return fmt.Errorf("error parsing '%v' params: key must not be "+
				"a blank string", self.Name())
		}
	}

	return nil
}

func (self *UpdateCommand) ExecuteUpdates(conf *model.TaskConfig) error {
	for _, update := range self.Updates {
		if update.Concat == "" {
			newValue, err := conf.Expansions.ExpandString(update.Value)

			if err != nil {
				return err
			}
			conf.Expansions.Put(update.Key, newValue)
		} else {
			newValue, err := conf.Expansions.ExpandString(update.Concat)
			if err != nil {
				return err
			}

			oldValue := conf.Expansions.Get(update.Key)
			conf.Expansions.Put(update.Key, oldValue+newValue)
		}
	}

	return nil
}

// Execute updates the expansions. Fulfills Command interface.
func (self *UpdateCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {

	err := self.ExecuteUpdates(conf)
	if err != nil {
		return err
	}

	if self.YamlFile != "" {
		pluginLogger.LogTask(slogger.INFO, "Updating expansions with keys from file: %v", self.YamlFile)
		filename := filepath.Join(conf.WorkDir, self.YamlFile)
		err := conf.Expansions.UpdateFromYaml(filename)
		if err != nil {
			return err
		}
	}
	return nil

}
