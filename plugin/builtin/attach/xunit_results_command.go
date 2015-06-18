package attach

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/attach/xunit"
	"github.com/mitchellh/mapstructure"
	"os"
	"path/filepath"
)

// AttachXUnitResultsCommand reads in an xml file of xunit
// type results and converts them to a format MCI can use
type AttachXUnitResultsCommand struct {
	// File describes the relative path of the file to be sent. Supports globbing.
	// Note that this can also be described via expansions.
	File string `mapstructure:"file" plugin:"expand"`
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
	if self.File == "" {
		return fmt.Errorf("file cannot be blank")
	}
	return nil
}

// Expand the parameter appropriately
func (self *AttachXUnitResultsCommand) expandParams(
	taskConfig *model.TaskConfig) (err error) {

	self.File, err = taskConfig.Expansions.ExpandString(self.File)
	if err != nil {
		return fmt.Errorf("error expanding path '%v': %v", self.File, err)
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

// getFilePaths is a helper function that returns a slice of all absolute paths
// which match the given file path parameters.
func (self *AttachXUnitResultsCommand) getFilePaths(
	workDir string) ([]string, error) {

	patternPath := filepath.Join(workDir, self.File)
	paths, err := filepath.Glob(patternPath)
	if err != nil {
		return nil, fmt.Errorf("file specified an incorrect pattern: '%v'", err)
	}
	return paths, nil
}

func (self *AttachXUnitResultsCommand) parseAndUploadResults(
	taskConfig *model.TaskConfig, pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator) error {
	tests := []model.TestResult{}
	logs := []*model.TestLog{}
	logIdxToTestIdx := []int{}

	reportFilePaths, err := self.getFilePaths(taskConfig.WorkDir)
	if err != nil {
		return err
	}

	for _, reportFileLoc := range reportFilePaths {
		file, err := os.Open(reportFileLoc)
		if err != nil {
			return fmt.Errorf("couldn't open xunit file: '%v'", err)
		}

		testSuites, err := xunit.ParseXMLResults(file)
		if err != nil {
			return fmt.Errorf("error parsing xunit file: '%v'", err)
		}

		err = file.Close()
		if err != nil {
			return fmt.Errorf("error closing xunit file: '%v'", err)
		}

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
