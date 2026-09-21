package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const maxCompletionBatchSize = 100

type completeVirtualTasks struct {
	Files    []string `mapstructure:"files" plugin:"expand"`
	Optional bool     `mapstructure:"optional"`
	base
}

func completeVirtualTasksFactory() Command   { return &completeVirtualTasks{} }
func (c *completeVirtualTasks) Name() string { return "virtual_tasks.complete" }

func (c *completeVirtualTasks) ParseParams(params map[string]any) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}
	if len(c.Files) == 0 {
		return errors.New("must provide at least 1 file containing virtual task completions")
	}
	return nil
}

func (c *completeVirtualTasks) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	include := utility.NewGitIgnoreFileMatcher(conf.WorkDir, c.Files...)
	b := utility.FileListBuilder{
		WorkingDir: conf.WorkDir,
		Include:    include,
	}
	var err error
	if c.Files, err = b.Build(); err != nil {
		return errors.Wrap(err, "building wildcard paths")
	}

	if len(c.Files) == 0 {
		if c.Optional {
			logger.Task().Infof(ctx, "No files found and optional is true, skipping command '%s'.", c.Name())
			return nil
		}
		return errors.Errorf("no files found for command '%s'", c.Name())
	}

	var allCompletions []apimodels.VirtualTaskCompletion
	catcher := grip.NewBasicCatcher()
	for _, fn := range c.Files {
		if ctx.Err() != nil {
			catcher.Wrapf(ctx.Err(), "cancelled before processing file '%s'", fn)
			break
		}
		completions, err := readVirtualTaskCompletionsFile(conf, fn)
		if err != nil {
			catcher.Add(err)
			continue
		}
		allCompletions = append(allCompletions, completions...)
	}
	if catcher.HasErrors() {
		return errors.WithStack(catcher.Resolve())
	}

	if len(allCompletions) == 0 {
		logger.Task().Warning(ctx, "No virtual task completions found in files.")
		return nil
	}

	logger.Task().Infof(ctx, "Completing %d virtual task(s).", len(allCompletions))

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	failCatcher := grip.NewBasicCatcher()
	for i := 0; i < len(allCompletions); i += maxCompletionBatchSize {
		end := min(i+maxCompletionBatchSize, len(allCompletions))
		batch := allCompletions[i:end]

		resp, err := comm.CompleteVirtualTasks(ctx, td, batch)
		if err != nil {
			return errors.Wrap(err, "completing virtual tasks")
		}

		for _, result := range resp.Results {
			if result.Outcome == apimodels.VirtualTaskCompletionOutcomeFailed {
				logger.Task().Errorf(ctx, "Virtual task '%s' completion failed: %s", result.TaskID, result.Reason)
				failCatcher.Errorf("virtual task '%s': %s", result.TaskID, result.Reason)
			} else if result.Reason != "" {
				logger.Task().Infof(ctx, "Virtual task '%s' completed successfully: %s", result.TaskID, result.Reason)
			} else {
				logger.Task().Infof(ctx, "Virtual task '%s' completed successfully.", result.TaskID)
			}
		}
	}

	if failCatcher.HasErrors() {
		return errors.Wrap(failCatcher.Resolve(), "some virtual tasks failed to complete")
	}

	return nil
}

func readVirtualTaskCompletionsFile(conf *internal.TaskConfig, fn string) ([]apimodels.VirtualTaskCompletion, error) {
	fileLoc := GetWorkingDirectory(conf, fn)
	f, err := os.Open(fileLoc)
	if err != nil {
		return nil, errors.Wrapf(err, "opening file '%s'", fn)
	}
	defer f.Close()

	var completions []apimodels.VirtualTaskCompletion
	if err := utility.ReadJSON(f, &completions); err != nil {
		return nil, errors.Wrapf(err, "reading JSON from file '%s'", fn)
	}
	return completions, nil
}
