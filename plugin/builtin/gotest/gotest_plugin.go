package gotest

import (
	"10gen.com/mci/plugin"
	"fmt"
	"net/http"
	"time"
)

func init() {
	plugin.Publish(&GotestPlugin{})
}

const (
	GotestPluginName     = "gotest"
	RunTestCommandName   = "run"
	ResultsAPIEndpoint   = "gotest_results"
	TestLogsAPIEndpoint  = "gotest_logs"
	ResultsPostRetries   = 5
	ResultsRetrySleepSec = 10 * time.Second
)

type GotestPlugin struct{}

func (self *GotestPlugin) Name() string {
	return GotestPluginName
}

func (self *GotestPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	switch cmdName {
	case RunTestCommandName:
		return &RunTestCommand{}, nil
	default:
		return nil, fmt.Errorf("No such %v command: %v", GotestPluginName, cmdName)
	}
}

func (self *GotestPlugin) Configure(map[string]interface{}) error {
	return nil
}

func (self *GotestPlugin) GetAPIHandler() http.Handler {
	return nil
}

func (self *GotestPlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *GotestPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return nil, nil
}
