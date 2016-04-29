package gotest

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/plugin"
	"time"
)

func init() {
	plugin.Publish(&GotestPlugin{})
}

const (
	GotestPluginName      = "gotest"
	ParseFilesCommandName = "parse_files"
	ResultsAPIEndpoint    = "gotest_results"
	TestLogsAPIEndpoint   = "gotest_logs"
	ResultsPostRetries    = 5
	ResultsRetrySleepSec  = 10 * time.Second
)

type GotestPlugin struct{}

func (self *GotestPlugin) Name() string {
	return GotestPluginName
}

func (self *GotestPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	switch cmdName {
	case ParseFilesCommandName:
		return &ParseFilesCommand{}, nil
	default:
		return nil, fmt.Errorf("No such %v command: %v", GotestPluginName, cmdName)
	}
}
