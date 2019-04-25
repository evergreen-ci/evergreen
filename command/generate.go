package command

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type generateTask struct {
	// Files are a list of JSON documents.
	Files []string `mapstructure:"files" plugin:"expand"`
	base
}

func generateTaskFactory() Command   { return &generateTask{} }
func (c *generateTask) Name() string { return "generate.tasks" }

func (c *generateTask) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "Error decoding %s params", c.Name())
	}
	if len(c.Files) == 0 {
		return errors.Errorf("Must provide at least 1 file to '%s'", c.Name())
	}
	return nil
}

func (c *generateTask) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	var err error
	if conf.Task.Execution > 0 {
		logger.Task().Warning("Refusing to generate tasks on an execution other than the first one")
		return nil
	}
	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding params")
	}

	if c.Files, err = util.BuildFileList(conf.WorkDir, c.Files...); err != nil {
		return errors.Wrap(err, "problem building wildcard paths")
	}

	if len(c.Files) == 0 {
		return errors.New("expanded file specification had no items")
	}

	catcher := grip.NewBasicCatcher()
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	var jsonBytes [][]byte
	for _, fn := range c.Files {
		if ctx.Err() != nil {
			catcher.Add(ctx.Err())
			break
		}
		var data []byte
		data, err = generateTaskForFile(fn, conf)
		if err != nil {
			catcher.Add(err)
			continue
		}
		jsonBytes = append(jsonBytes, data)
	}
	if catcher.HasErrors() {
		return errors.WithStack(catcher.Resolve())
	}

	var post []json.RawMessage
	post, err = makeJsonOfAllFiles(jsonBytes)
	if err != nil {
		return errors.Wrap(err, "problem parsing JSON")
	}
	if err = comm.GenerateTasks(ctx, td, post); err != nil {
		return errors.Wrap(err, "Problem posting task data")
	}

	var generateStatus *apimodels.GeneratePollResponse
	err = util.Retry(
		ctx,
		func() (bool, error) {
			generateStatus, err = comm.GenerateTasksPoll(ctx, td)
			if err != nil {
				return true, err
			}
			if generateStatus.Finished {
				return false, nil
			}
			return true, errors.New("task generation unfinished")
		}, 100, time.Second, 15*time.Second)
	if err != nil {
		return errors.WithMessage(err, "problem polling for generate tasks job")
	}
	if len(generateStatus.Errors) > 0 {
		return errors.New(strings.Join(generateStatus.Errors, ", "))
	}
	return nil
}

func generateTaskForFile(fn string, conf *model.TaskConfig) ([]byte, error) {
	fileLoc := filepath.Join(conf.WorkDir, fn)
	if _, err := os.Stat(fileLoc); os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "File '%s' does not exist", fn)
	}
	jsonFile, err := os.Open(fileLoc)
	if err != nil {
		return nil, errors.Wrapf(err, "Couldn't open file '%s'", fn)
	}
	defer jsonFile.Close()

	var data []byte
	data, err = ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Problem reading from file '%s'", fn)
	}

	return data, nil
}

// makeJsonOfAllFiles creates a single JSON document that is an array of all JSON files. This allows
// us to avoid posting multiple JSON files.
func makeJsonOfAllFiles(jsonBytes [][]byte) ([]json.RawMessage, error) {
	catcher := grip.NewBasicCatcher()
	post := []json.RawMessage{}
	for _, j := range jsonBytes {
		jsonRaw := json.RawMessage{}
		if err := json.Unmarshal(j, &jsonRaw); err != nil {

			catcher.Add(errors.Wrap(err, "error unmarshaling JSON for generate.tasks"))
			continue
		}
		post = append(post, jsonRaw)
	}
	return post, catcher.Resolve()
}
