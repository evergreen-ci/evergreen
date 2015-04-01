package attach

import (
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"10gen.com/mci/plugin/builtin/attach/xunit"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/mapstructure"
	"os"
	"path/filepath"
)

// AttachXUnitResultsCommand reads in an xml file of xunit
// type results and converts them to a format MCI can use
type AttachXUnitResultsCommand struct {
	// FileLoc describes the relative path of the file to be sent.
	// Note that this can also be described via expansions.
	FileLoc string `mapstructure:"file_location" plugin:"expand"`
}

func (self *AttachXUnitResultsCommand) Name() string {
	return AttachXunitResultsCmd
}

func (self *AttachXUnitResultsCommand) Plugin() string {
	return AttachPluginName
}

// ParseParams reads and validates the command parameters. This is required
// to satisfy the 'Command' interface
func (self *AttachXUnitResultsCommand) ParseParams(
	params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", self.Name(), err)
	}
	if err := self.validateParams(); err != nil {
		return fmt.Errorf("error validating '%v' params: %v", self.Name(), err)
	}
	return nil
}

// validateParams is a helper function that ensures all
// the fields necessary for attaching an xunit results are present
func (self *AttachXUnitResultsCommand) validateParams() (err error) {
	if self.FileLoc == "" {
		return fmt.Errorf("file_location cannot be blank")
	}
	return nil
}

func (self *AttachXUnitResultsCommand) expandParams(
	taskConfig *model.TaskConfig) (err error) {
	self.FileLoc, err = taskConfig.Expansions.ExpandString(self.FileLoc)
	if err != nil {
		return fmt.Errorf("error expanding file_location '%v': %v", self.FileLoc, err)
	}
	return nil
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (self *AttachXUnitResultsCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	taskConfig *model.TaskConfig,
	stop chan bool) error {

	if err := self.expandParams(taskConfig); err != nil {
		return err
	}

	errChan := make(chan error)
	go func() {
		errChan <- self.parseAndUploadResults(taskConfig, pluginLogger, pluginCom)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of attach xunit results command")
		return nil
	}
}

func (self *AttachXUnitResultsCommand) parseAndUploadResults(
	taskConfig *model.TaskConfig, pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator) error {
	tests := []model.TestResult{}
	logs := []*model.TestLog{}

	reportFileLoc := filepath.Join(taskConfig.WorkDir, self.FileLoc)
	file, err := os.Open(reportFileLoc)
	if err != nil {
		return fmt.Errorf("couldn't open xunit file: '%v'", err)
	}

	testSuites, err := xunit.ParseXMLResults(file)
	if err != nil {
		return fmt.Errorf("error parsing xunit file: '%v'", err)
	}

	logIdxToTestIdx := []int{}
	// go through all the tests
	for _, suite := range testSuites {
		for _, tc := range suite.TestCases {
			// logs are only created when a test case does not succeed
			test, log := tc.ToModelTestResultAndLog(taskConfig.Task)
			if log != nil {
				logs = append(logs, log)
				logIdxToTestIdx = append(logIdxToTestIdx, len(tests))
			}
			tests = append(tests, test)
		}
	}

	for i, log := range logs {
		logId, err := SendJSONLogs(taskConfig, pluginLogger, pluginCom, log)
		if err != nil {
			pluginLogger.LogTask(slogger.WARN, "Error uploading logs for %v", log.Name)
			continue
		}
		tests[logIdxToTestIdx[i]].LogId = logId
		tests[logIdxToTestIdx[i]].LineNum = 1
	}

	return SendJSONResults(taskConfig, pluginLogger, pluginCom, &model.TestResults{tests})
}
