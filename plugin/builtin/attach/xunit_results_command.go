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
	// FileLoc describes the relative path of the file to be sent.
	// Note that this can also be described via expansions.
	FileLoc string `mapstructure:"file_location" plugin:"expand"`
	// FilePattern describes the relative path of any number of files
	// that fit the pattern.
	FilePattern string `mapstructure:"file_pattern" plugin:"expand"`
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
	if (self.FileLoc == "" && self.FilePattern == "") ||
		(self.FileLoc != "" && self.FilePattern != "") {
		return fmt.Errorf("please specify file_location (X)OR file_pattern")
	}
	return nil
}

// Given any string, expand appropriately or error if not possible
func expandParamString(fileString *string, taskConfig *model.TaskConfig) (
	err error) {

	*fileString, err = taskConfig.Expansions.ExpandString(*fileString)
	if err != nil {
		return fmt.Errorf("error expanding path '%v': %v", *fileString, err)
	}
	return nil
}

// Expand the appropriate parameter
func (self *AttachXUnitResultsCommand) expandParams(
	taskConfig *model.TaskConfig) (err error) {
	if self.FileLoc != "" {
		return expandParamString(&self.FileLoc, taskConfig)
	}
	return expandParamString(&self.FilePattern, taskConfig)
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
	taskConfig *model.TaskConfig) []string {

	if self.FileLoc != "" {
		return []string{filepath.Join(taskConfig.WorkDir, self.FileLoc)}
	}

	patternPath := filepath.Join(taskConfig.WorkDir, self.FilePattern)
	paths, err := filepath.Glob(patternPath)
	if err != nil {
		fmt.Errorf("location_pattern specified an incorrect pattern: '%v'", err)
	}
	fmt.Printf("Paths: %v\n", paths)
	return paths
}

func (self *AttachXUnitResultsCommand) parseAndUploadResults(
	taskConfig *model.TaskConfig, pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator) error {
	tests := []model.TestResult{}
	logs := []*model.TestLog{}
	logIdxToTestIdx := []int{}

	reportFilePaths := self.getFilePaths(taskConfig)
	for _, reportFileLoc := range reportFilePaths {
		file, err := os.Open(reportFileLoc)
		if err != nil {
			return fmt.Errorf("couldn't open xunit file: '%v'", err)
		}

		testSuites, err := xunit.ParseXMLResults(file)
		if err != nil {
			return fmt.Errorf("error parsing xunit file: '%v'", err)
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
