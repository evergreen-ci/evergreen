package attach

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

// AttachResultsCommand is used to attach MCI test results in json
// format to the task page.
type AttachResultsCommand struct {
	// FileLoc describes the relative path of the file to be sent.
	// Note that this can also be described via expansions.
	FileLoc string `mapstructure:"file_location" plugin:"expand"`
}

func (self *AttachResultsCommand) Name() string {
	return AttachResultsCmd
}

func (self *AttachResultsCommand) Plugin() string {
	return AttachPluginName
}

// ParseParams decodes the S3 push command parameters that are
// specified as part of an AttachPlugin command; this is required
// to satisfy the 'Command' interface
func (self *AttachResultsCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", self.Name(), err)
	}
	if err := self.validateAttachResultsParams(); err != nil {
		return fmt.Errorf("error validating '%v' params: %v", self.Name(), err)
	}
	return nil
}

// validateAttachResultsParams is a helper function that ensures all
// the fields necessary for attaching a results are present
func (self *AttachResultsCommand) validateAttachResultsParams() (err error) {
	if self.FileLoc == "" {
		return fmt.Errorf("file_location cannot be blank")
	}
	return nil
}

func (self *AttachResultsCommand) expandAttachResultsParams(
	taskConfig *model.TaskConfig) (err error) {
	self.FileLoc, err = taskConfig.Expansions.ExpandString(self.FileLoc)
	if err != nil {
		return fmt.Errorf("error expanding file_location: %v", err)
	}
	return nil
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (self *AttachResultsCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	taskConfig *model.TaskConfig,
	stop chan bool) error {

	if err := self.expandAttachResultsParams(taskConfig); err != nil {
		return err
	}

	errChan := make(chan error)
	go func() {
		// attempt to open the file
		reportFileLoc := filepath.Join(taskConfig.WorkDir, self.FileLoc)
		reportFile, err := os.Open(reportFileLoc)
		if err != nil {
			errChan <- fmt.Errorf("Couldn't open report file: '%v'", err)
			return
		}
		results := &task.TestResults{}
		if err = util.ReadJSONInto(reportFile, results); err != nil {
			errChan <- fmt.Errorf("Couldn't read report file: '%v'", err)
			return
		}
		if err := reportFile.Close(); err != nil {
			pluginLogger.LogExecution(slogger.INFO, "Error closing file: %v", err)
		}
		errChan <- SendJSONResults(taskConfig, pluginLogger, pluginCom, results)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of attach results command")
		return nil
	}
}

// SendJSONResults is responsible for sending the
// specified file to the API Server
func SendJSONResults(taskConfig *model.TaskConfig,
	pluginLogger plugin.Logger, pluginCom plugin.PluginCommunicator,
	results *task.TestResults) error {
	for i, res := range results.Results {

		if res.LogRaw != "" {
			pluginLogger.LogExecution(slogger.INFO, "Attaching raw test logs")
			testLogs := &model.TestLog{
				Name:          res.TestFile,
				Task:          taskConfig.Task.Id,
				TaskExecution: taskConfig.Task.Execution,
				Lines:         []string{res.LogRaw},
			}

			id, err := pluginCom.TaskPostTestLog(testLogs)
			if err != nil {
				pluginLogger.LogExecution(slogger.ERROR, "Error posting raw logs from results: %v", err)
			} else {
				results.Results[i].LogId = id
			}

			// clear the logs from the TestResult struct after it has been saved in the test logs. Since they are
			// being saved in the test_logs collection, we can clear them to prevent them from being saved in the task
			// collection.
			results.Results[i].LogRaw = ""

		}
	}

	pluginLogger.LogExecution(slogger.INFO, "Attaching test results")
	err := pluginCom.TaskPostResults(results)
	if err != nil {
		return err
	}

	pluginLogger.LogTask(slogger.INFO, "Attach test results succeeded")
	return nil
}
