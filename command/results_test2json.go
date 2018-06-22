package command

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type test2JSONTestEvent struct {
	Time    time.Time // encodes as an RFC3339-format string
	Action  string
	Package string
	Test    string
	Elapsed float64 // seconds
	Output  string
}

type goTest2JSONCommand struct {
	Files []string `mapstructure:"files" plugin:"expand"`

	base
}

func goTest2JSONFactory() Command          { return &goTest2JSONCommand{} }
func (c *goTest2JSONCommand) Name() string { return "gotest.parse_json" }

func (c *goTest2JSONCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if len(c.Files) == 0 {
		return errors.Errorf("error validating params: must specify at least one "+
			"file pattern to parse: '%+v'", params)
	}
	return nil
}

func (c *goTest2JSONCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Task().Info("Sending test logs to server...")

	logger.Task().Info("Sending test logs to server...")

	return nil
}

func (c *goTest2JSONCommand) file(file string, logger client.LoggerProducer, conf *model.TaskConfig) error {
	path := file
	if !path.IsAbs(file) {
		path = path.Join(conf.WorkDir, file)
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Task().Errorf("Failed to open '%s'", path)
		return errors.Wrapf(err, "failed to open: %s", path)
	}

	lines := bytes.Split(data, "\n")
	for i := range lines {
		if len(lines[i] == 0) {
			continue
		}

		test2JSON, err := parseTest2JSON(lines[i])
		if err != nil {
			logger.Task().Error("failed to parse %s:%d", path, i+1)
			continue
		}

	}

}

func parseTest2JSON(bytes []byte) (*test2JSONTestEvent, error) {
	t := test2JSONTestEvent{}
	if err := json.Unmarshal(bytes, &t); err != nil {
		return nil, errors.Wrap(err, "failed to parse test2json")
	}
	return &t, nil
}

func test2JSONToTestResult(test2JSON *test2JSONTestEvent) task.TestResult {
	Status    string  `json:"status" bson:"status"`
	TestFile  string  `json:"test_file" bson:"test_file"`
	URL       string  `json:"url" bson:"url,omitempty"`
	URLRaw    string  `json:"url_raw" bson:"url_raw,omitempty"`
	LogId     string  `json:"log_id,omitempty" bson:"log_id,omitempty"`
	LineNum   int     `json:"line_num,omitempty" bson:"line_num,omitempty"`
	ExitCode  int     `json:"exit_code" bson:"exit_code"`
	StartTime float64 `json:"start" bson:"start"`
	EndTime   float64 `json:"end" bson:"end"`


	t := task.TestResult{
	    Status: evergreen.TestFailedStatus,
	    TestFile: test2JSON.Test,
	}

	log := model.TestLog{
	    Name: test2JSON.Test,
	    Lines: test2JSON.Output,
	}
}
