package expansions

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

const FetchVarsRoute = "fetch_vars"
const FetchVarsCmdname = "fetch"

// FetchVarsCommand pulls a set of vars (stored in the DB on the server side)
// and updates the agent's expansions map using the values it gets back
type FetchVarsCommand struct {
	Keys []FetchCommandParams `mapstructure:"keys" json:"keys"`
}

// FetchCommandParams is a pairing of remote key and local key values
type FetchCommandParams struct {
	// RemoteKey indicates which key in the projects vars map to use as the lvalue
	RemoteKey string `mapstructure:"remote_key" json:"remote_key"`

	// LocalKey indicates which key in the local expansions map to use as the rvalue
	LocalKey string `mapstructure:"local_key" json:"local_key"`
}

func (self *FetchVarsCommand) Name() string {
	return FetchVarsCmdname
}

func (self *FetchVarsCommand) Plugin() string {
	return ExpansionsPluginName
}

// ParseParams reads in the command's config. Fulfills the Command interface.
func (self *FetchVarsCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, self)
	if err != nil {
		return err
	}

	for _, item := range self.Keys {
		if item.RemoteKey == "" {
			return errors.Errorf("error parsing '%v' params: value for remote "+
				"key must not be a blank string", self.Name())
		}
		if item.LocalKey == "" {
			return errors.Errorf("error parsing '%v' params: value for local "+
				"key must not be a blank string", self.Name())
		}
	}
	return nil
}

// Execute fetches the expansions from the API server
func (self *FetchVarsCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	conf *model.TaskConfig,
	stop chan bool) error {

	pluginLogger.LogTask(slogger.ERROR, "Expansions.fetch deprecated")
	return nil

}
