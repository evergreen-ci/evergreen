package attach

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/attach/xunit"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

// AttachXUnitResultsCommand reads in an xml file of xunit
// type results and converts them to a format MCI can use
type AttachXUnitResultsCommand struct {
	// File describes the relative path of the file to be sent. Supports globbing.
	// Note that this can also be described via expansions.
	File  string   `mapstructure:"file" plugin:"expand"`
	Files []string `mapstructure:"files" plugin:"expand"`
}

func (c *AttachXUnitResultsCommand) Name() string {
	return AttachXunitResultsCmd
}

func (c *AttachXUnitResultsCommand) Plugin() string {
	return AttachPluginName
}

// ParseParams reads and validates the command parameters. This is required
// to satisfy the 'Command' interface
func (c *AttachXUnitResultsCommand) ParseParams(
	params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if err := c.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating '%s' params", c.Name())
	}

	return nil
}

// validateParams is a helper function that ensures all
// the fields necessary for attaching an xunit results are present
func (c *AttachXUnitResultsCommand) validateParams() (err error) {
	if c.File == "" && len(c.Files) == 0 {
		return errors.New("must specify at least one file")
	}
	return nil
}

// Expand the parameter appropriately
func (c *AttachXUnitResultsCommand) expandParams(conf *model.TaskConfig) error {
	if c.File != "" {
		c.Files = append(c.Files, c.File)
	}

	var err error
	catcher := grip.NewCatcher()

	for idx, f := range c.Files {
		c.Files[idx], err = conf.Expansions.ExpandString(f)
		catcher.Add(err)
	}

	return errors.Wrapf(catcher.Resolve(), "problem expanding paths")
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (c *AttachXUnitResultsCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	taskConfig *model.TaskConfig,
	stop chan bool) error {

	if err := c.expandParams(taskConfig); err != nil {
		return err
	}

	errChan := make(chan error)
	go func() {
		errChan <- c.parseAndUploadResults(taskConfig, pluginLogger, pluginCom)
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
func getFilePaths(workDir string, files []string) ([]string, error) {
	catcher := grip.NewCatcher()
	out := []string{}

	for _, fileSpec := range files {
		paths, err := filepath.Glob(filepath.Join(workDir, fileSpec))
		catcher.Add(err)
		out = append(out, paths...)
	}

	return out, errors.Wrapf(catcher.Resolve(), "%d incorrect file specifications", catcher.Len())
}

func (c *AttachXUnitResultsCommand) parseAndUploadResults(
	taskConfig *model.TaskConfig, pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator) error {
	tests := []task.TestResult{}
	logs := []*model.TestLog{}
	logIdxToTestIdx := []int{}

	reportFilePaths, err := getFilePaths(taskConfig.WorkDir, c.Files)
	if err != nil {
		return err
	}

	for _, reportFileLoc := range reportFilePaths {
		file, err := os.Open(reportFileLoc)
		if err != nil {
			return errors.Wrap(err, "couldn't open xunit file")
		}

		testSuites, err := xunit.ParseXMLResults(file)
		if err != nil {
			return errors.Wrap(err, "error parsing xunit file")
		}

		if err = file.Close(); err != nil {
			return errors.Wrap(err, "error closing xunit file")
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
		logId, err := SendJSONLogs(pluginLogger, pluginCom, log)
		if err != nil {
			pluginLogger.LogTask(slogger.WARN, "Error uploading logs for %v", log.Name)
			continue
		}
		tests[logIdxToTestIdx[i]].LogId = logId
		tests[logIdxToTestIdx[i]].LineNum = 1
	}

	return SendJSONResults(taskConfig, pluginLogger, pluginCom, &task.TestResults{tests})
}
