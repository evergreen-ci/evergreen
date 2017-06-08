package keyval

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	KeyValCollection = "keyval_plugin"
	KeyValPluginName = "keyval"
	IncCommandName   = "inc"
	IncRoute         = "inc"
)

type KeyVal struct {
	Key   string `bson:"_id" json:"key"`
	Value int64  `bson:"value" json:"value"`
}

func init() {
	plugin.Publish(&KeyValPlugin{})
}

type KeyValPlugin struct{}

func (self *KeyValPlugin) Configure(map[string]interface{}) error {
	return nil
}

func (self *KeyValPlugin) Name() string {
	return KeyValPluginName
}

type IncCommand struct {
	Key         string `mapstructure:"key"`
	Destination string `mapstructure:"destination"`
}

func (self *IncCommand) Name() string {
	return IncCommandName
}

func (self *IncCommand) Plugin() string {
	return KeyValPluginName
}

// ParseParams validates the input to the IncCommand, returning an error
// if something is incorrect. Fulfills Command interface.
func (incCmd *IncCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, incCmd)
	if err != nil {
		return err
	}

	if incCmd.Key == "" || incCmd.Destination == "" {
		return fmt.Errorf("error parsing '%v' params: key and destination may not be blank",
			IncCommandName)
	}

	return nil
}

// GetAPIHandler returns the routes to be bound by the API server
func (self *KeyValPlugin) GetAPIHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc("/inc", IncKeyHandler)
	r.HandleFunc("/", http.NotFound)
	return r
}

// IncKeyHandler increments the value stored in the given key, and returns it
func IncKeyHandler(w http.ResponseWriter, r *http.Request) {
	key := ""
	err := util.ReadJSONInto(r.Body, &key)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error geting key: %v", err)
		plugin.WriteJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	change := mgo.Change{
		Update: bson.M{
			"$inc": bson.M{"value": 1},
		},
		ReturnNew: true,
		Upsert:    true,
	}

	keyVal := &KeyVal{}
	_, err = db.FindAndModify(KeyValCollection, bson.M{"_id": key}, nil, change, keyVal)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "error doing findAndModify: %v", err)
		plugin.WriteJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	plugin.WriteJSON(w, http.StatusOK, keyVal)
}

// Execute fetches the expansions from the API server
func (incCmd *IncCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig,
	stop chan bool) error {

	err := plugin.ExpandValues(incCmd, conf.Expansions)
	if err != nil {
		return err
	}

	keyVal := &KeyVal{}
	postFunc := func() error {
		resp, err := pluginCom.TaskPostJSON(IncRoute, incCmd.Key)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			return util.RetriableError{err}
		}
		if resp.StatusCode != http.StatusOK {
			return util.RetriableError{
				fmt.Errorf("unexpected status code: %v", resp.StatusCode),
			}
		}
		err = util.ReadJSONInto(resp.Body, keyVal)
		if err != nil {
			return fmt.Errorf("failed to read JSON reply: %v", err)
		}
		return nil
	}

	retryFail, err := util.Retry(postFunc, 10, 1*time.Second)
	if retryFail {
		return fmt.Errorf("incrementing value failed after %v tries: %v", 10, err)
	}
	if err != nil {
		return err
	}

	conf.Expansions.Put(incCmd.Destination, fmt.Sprintf("%d", keyVal.Value))
	return nil
}

func (self *KeyValPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == IncCommandName {
		return &IncCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}
