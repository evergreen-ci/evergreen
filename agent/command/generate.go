package command

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type generateTask struct {
	// Files are a list of JSON documents.
	Files []string `mapstructure:"files" plugin:"expand"`

	// Optional causes generate.tasks to noop if no files match
	Optional bool `mapstructure:"optional"`

	base
}

func generateTaskFactory() Command   { return &generateTask{} }
func (c *generateTask) Name() string { return "generate.tasks" }

func (c *generateTask) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}
	if len(c.Files) == 0 {
		return errors.Errorf("must provide at least 1 file containing task generation definitions")
	}
	return nil
}

func (c *generateTask) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error
	if err = util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	include := utility.NewGitIgnoreFileMatcher(conf.WorkDir, c.Files...)
	b := utility.FileListBuilder{
		WorkingDir: conf.WorkDir,
		Include:    include,
	}
	if c.Files, err = b.Build(); err != nil {
		return errors.Wrap(err, "building wildcard paths")
	}

	if len(c.Files) == 0 {
		if c.Optional {
			logger.Task().Infof("No files found and optional is true, skipping command '%s'.", c.Name())
			return nil
		}
		return errors.Errorf("no files found for command '%s'", c.Name())
	}

	catcher := grip.NewBasicCatcher()
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	var jsonBytes [][]byte
	for _, fn := range c.Files {
		if err := ctx.Err(); err != nil {
			catcher.Wrapf(ctx.Err(), "cancelled while processing file '%s'", fn)
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
		return errors.Wrap(err, "parsing JSON")
	}
	if err = comm.GenerateTasks(ctx, td, post); err != nil {
		if strings.Contains(err.Error(), evergreen.TasksAlreadyGeneratedError) {
			logger.Task().Info("Tasks have already been generated, nooping.")
			return nil
		}
		return errors.Wrap(err, "posting task data")
	}

	const (
		pollAttempts      = 1500
		pollRetryMinDelay = time.Second
		pollRetryMaxDelay = 15 * time.Second
	)

	err = utility.Retry(
		ctx,
		func() (bool, error) {
			generateStatus, err := comm.GenerateTasksPoll(ctx, td)
			if err != nil {
				return false, err
			}

			var generateErr error
			if generateStatus.Error != "" {
				generateErr = errors.New(generateStatus.Error)
			}

			if generateErr != nil {
				return false, generateErr
			}
			if generateStatus.Finished {
				return false, nil
			}
			return true, errors.New("task generation unfinished")
		}, utility.RetryOptions{
			MaxAttempts: pollAttempts,
			MinDelay:    pollRetryMinDelay,
			MaxDelay:    pollRetryMaxDelay,
		})
	if err != nil {
		return errors.WithMessage(err, "polling for generate tasks job")
	}
	return nil
}

func generateTaskForFile(fn string, conf *internal.TaskConfig) ([]byte, error) {
	fileLoc := GetWorkingDirectory(conf, fn)
	if _, err := os.Stat(fileLoc); os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "getting information for file '%s'", fn)
	}
	jsonFile, err := os.Open(fileLoc)
	if err != nil {
		return nil, errors.Wrapf(err, "opening file '%s'", fn)
	}
	defer jsonFile.Close()

	var data []byte
	data, err = io.ReadAll(jsonFile)
	if err != nil {
		return nil, errors.Wrapf(err, "reading from file '%s'", fn)
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
			catcher.Wrap(err, "unmarshalling JSON from file")
			continue
		}
		post = append(post, jsonRaw)
	}
	return post, catcher.Resolve()
}
