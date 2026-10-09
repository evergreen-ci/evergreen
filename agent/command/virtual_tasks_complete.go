package command

import (
	"context"
	"os"
	"time"

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

// virtualTaskCompletionFile is a single virtual task completion as read from a
// completion file.
type virtualTaskCompletionFile struct {
	TaskID           string                               `json:"task_id"`
	Execution        int                                  `json:"execution"`
	Status           string                               `json:"status"`
	TestResults      *virtualTaskTestResultsFile          `json:"test_results,omitempty"`
	Artifacts        []apimodels.VirtualTaskArtifact      `json:"artifacts,omitempty"`
	ExternalMetadata *apimodels.ExternalExecutionMetadata `json:"external_metadata,omitempty"`
}

// virtualTaskTestResultsFile describes the test result files to parse and upload
// for a virtual task.
type virtualTaskTestResultsFile struct {
	Files     []virtualTaskTestResultFileGroup `json:"files"`
	CreatedAt time.Time                        `json:"created_at,omitempty"`
}

// virtualTaskTestResultFileGroup is a group of files of a single test result type.
type virtualTaskTestResultFileGroup struct {
	Type  string   `json:"type"`
	Files []string `json:"files"`
}

func (tr virtualTaskTestResultsFile) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(tr.Files) == 0, "test_results must specify at least one file group")
	for _, group := range tr.Files {
		catcher.ErrorfWhen(len(group.Files) == 0, "test result file group '%s' must specify at least one file", group.Type)
		switch group.Type {
		case virtualTestResultTypeNative, virtualTestResultTypeGo, virtualTestResultTypeXUnit:
		default:
			catcher.Errorf("unrecognized test result type '%s'", group.Type)
		}
	}
	return catcher.Resolve()
}

// toCompletion converts a virtual task's completion file data to the API model
// for push-completing the task.
func (e virtualTaskCompletionFile) toCompletion(tr *apimodels.VirtualTaskTestResults) apimodels.VirtualTaskCompletion {
	return apimodels.VirtualTaskCompletion{
		TaskID:           e.TaskID,
		Execution:        e.Execution,
		Status:           e.Status,
		Artifacts:        e.Artifacts,
		ExternalMetadata: e.ExternalMetadata,
		TestResults:      tr,
	}
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

	var entries []virtualTaskCompletionFile
	catcher := grip.NewBasicCatcher()
	for _, fn := range c.Files {
		if ctx.Err() != nil {
			catcher.Wrapf(ctx.Err(), "cancelled before processing file '%s'", fn)
			break
		}
		fileEntries, err := readVirtualTaskCompletionsFile(conf, fn)
		if err != nil {
			catcher.Add(err)
			continue
		}
		entries = append(entries, fileEntries...)
	}
	if catcher.HasErrors() {
		return errors.WithStack(catcher.Resolve())
	}

	if len(entries) == 0 {
		logger.Task().Warning(ctx, "No virtual task completions found in files.")
		return nil
	}

	logger.Task().Infof(ctx, "Completing %d virtual task(s).", len(entries))

	completions, err := buildVirtualTaskCompletions(ctx, comm, logger, conf, entries)
	catcher.Wrap(err, "building virtual task completions")
	if len(completions) > 0 {
		catcher.Wrap(completeVirtualTaskBatches(ctx, comm, logger, conf, completions), "push-completing virtual tasks")
	}

	return errors.WithStack(catcher.Resolve())
}

// buildVirtualTaskCompletions builds the completions to push for the given
// entries, uploading test results first for the entries that specify them. Tasks
// that fail to prepare or upload are logged and skipped so the remaining tasks
// can still be completed.
func buildVirtualTaskCompletions(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, entries []virtualTaskCompletionFile) ([]apimodels.VirtualTaskCompletion, error) {
	catcher := grip.NewBasicCatcher()

	withoutTestResults, withTestResults, err := partitionCompletions(entries)
	catcher.Wrap(err, "partitioning virtual task completions")

	uploaded, err := uploadTestResultsForCompletions(ctx, comm, logger, conf, withTestResults)
	catcher.Wrap(err, "uploading virtual task test results")

	return append(withoutTestResults, uploaded...), catcher.Resolve()
}

// partitionCompletions splits entries into completions that have no test
// results to upload and those with test results to upload. Tasks that are
// duplicate or contain invalid test results are dropped and reported as errors.
func partitionCompletions(entries []virtualTaskCompletionFile) (withoutTestResults []apimodels.VirtualTaskCompletion, withTestResults []virtualTaskCompletionFile, err error) {
	catcher := grip.NewBasicCatcher()
	withoutTestResults = make([]apimodels.VirtualTaskCompletion, 0, len(entries))
	seenTestResults := map[string]bool{}
	for _, entry := range entries {
		if entry.TestResults == nil {
			withoutTestResults = append(withoutTestResults, entry.toCompletion(nil))
			continue
		}
		if err := entry.TestResults.validate(); err != nil {
			catcher.Wrapf(err, "virtual task '%s'", entry.TaskID)
			continue
		}
		if seenTestResults[entry.TaskID] {
			catcher.Errorf("virtual task '%s' specifies test results more than once", entry.TaskID)
			continue
		}
		seenTestResults[entry.TaskID] = true
		withTestResults = append(withTestResults, entry)
	}
	return withoutTestResults, withTestResults, catcher.Resolve()
}

// uploadTestResultsForCompletions prepares each entry's virtual task, uploads its
// test results, and returns the completions to push. Tasks that fail to prepare
// or upload are logged and skipped.
func uploadTestResultsForCompletions(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, entries []virtualTaskCompletionFile) ([]apimodels.VirtualTaskCompletion, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	catcher := grip.NewBasicCatcher()

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	prepared, err := prepareVirtualTasks(ctx, comm, td, logger, entries)
	catcher.Wrap(err, "preparing virtual tasks for test result upload")

	completions := make([]apimodels.VirtualTaskCompletion, 0, len(entries))
	for _, entry := range entries {
		prep, ok := prepared[entry.TaskID]
		if !ok {
			// The task could not be prepared for uploading test results, so it
			// cannot have its test results safely pushed.
			logger.Task().Warningf(ctx, "Skipping test results for virtual task '%s' because it could not be prepared for upload", entry.TaskID)
			continue
		}
		if prep.Reason != "" {
			logger.Task().Infof(ctx, "Skipping test result upload for virtual task '%s': %s", entry.TaskID, prep.Reason)
			continue
		}

		completion, err := uploadEntryTestResults(ctx, logger, conf, entry, prep)
		if err != nil {
			logger.Task().Errorf(ctx, "Test result upload for virtual task '%s' failed: %s", entry.TaskID, err)
			catcher.Errorf("virtual task '%s': %s", entry.TaskID, err)
			continue
		}
		completions = append(completions, *completion)
	}
	return completions, catcher.Resolve()
}

// uploadEntryTestResults uploads the test results for a virtual task and
// returns its push-completion data, including test result stats.
func uploadEntryTestResults(ctx context.Context, logger client.LoggerProducer, conf *internal.TaskConfig, entry virtualTaskCompletionFile, prep apimodels.VirtualTaskPreparationResult) (*apimodels.VirtualTaskCompletion, error) {
	testResultsCreatedAt := entry.TestResults.CreatedAt
	if utility.IsZeroTime(testResultsCreatedAt) {
		testResultsCreatedAt = time.Now()
	}
	stats, err := processVirtualTaskTestResultFiles(ctx, conf, logger, *prep.TestResultsInfo, prep.TaskCreateTime, testResultsCreatedAt, entry.TestResults.Files)
	if err != nil {
		return nil, err
	}

	completion := entry.toCompletion(stats)
	return &completion, nil
}

// prepareVirtualTasks prepares a virtual task to upload test results by
// validating and locking the given virtual tasks. If the preparation succeeds
// and the task is ready to have test results uploaded, it returns all the
// prepared virtual tasks and their task info.
func prepareVirtualTasks(ctx context.Context, comm client.Communicator, td client.TaskData, logger client.LoggerProducer, entries []virtualTaskCompletionFile) (map[string]apimodels.VirtualTaskPreparationResult, error) {
	preparations := make([]apimodels.VirtualTaskPreparation, len(entries))
	for i, entry := range entries {
		preparations[i] = apimodels.VirtualTaskPreparation{
			TaskID:    entry.TaskID,
			Execution: entry.Execution,
		}
	}

	catcher := grip.NewBasicCatcher()
	prepared := map[string]apimodels.VirtualTaskPreparationResult{}
	for i := 0; i < len(preparations); i += maxCompletionBatchSize {
		end := min(i+maxCompletionBatchSize, len(preparations))
		batch := preparations[i:end]

		resp, err := comm.PrepareVirtualTasks(ctx, td, batch)
		if err != nil {
			catcher.Wrapf(err, "preparing virtual tasks (batch between indexes %d-%d)", i, end)
			continue
		}
		for _, result := range resp.Results {
			switch result.Outcome {
			case apimodels.VirtualTaskCompletionOutcomeFailed:
				logger.Task().Errorf(ctx, "Virtual task '%s' preparation failed: %s", result.TaskID, result.Reason)
				catcher.Errorf("virtual task '%s': %s", result.TaskID, result.Reason)
			case apimodels.VirtualTaskCompletionOutcomeSuccess:
				prepared[result.TaskID] = result
			default:
				catcher.Errorf("virtual task '%s': unrecognized preparation outcome '%s'", result.TaskID, result.Outcome)
			}
		}
	}
	return prepared, catcher.Resolve()
}

// completeVirtualTaskBatches push-completes the given virtual tasks in batches.
func completeVirtualTaskBatches(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, completions []apimodels.VirtualTaskCompletion) error {
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	catcher := grip.NewBasicCatcher()
	for i := 0; i < len(completions); i += maxCompletionBatchSize {
		end := min(i+maxCompletionBatchSize, len(completions))
		batch := completions[i:end]

		resp, err := comm.CompleteVirtualTasks(ctx, td, batch)
		if err != nil {
			catcher.Wrapf(err, "completing virtual tasks (batch between indexes %d-%d)", i, end)
			continue
		}

		for _, result := range resp.Results {
			if result.Outcome == apimodels.VirtualTaskCompletionOutcomeFailed {
				logger.Task().Errorf(ctx, "Virtual task '%s' completion failed: %s", result.TaskID, result.Reason)
				catcher.Errorf("virtual task '%s': %s", result.TaskID, result.Reason)
			} else if result.Reason != "" {
				logger.Task().Infof(ctx, "Skipping push-completion for virtual task '%s', no-opping: %s", result.TaskID, result.Reason)
			} else {
				logger.Task().Infof(ctx, "Virtual task '%s' completed successfully.", result.TaskID)
			}
		}
	}
	return catcher.Resolve()
}

func readVirtualTaskCompletionsFile(conf *internal.TaskConfig, fn string) ([]virtualTaskCompletionFile, error) {
	fileLoc := GetWorkingDirectory(conf, fn)
	f, err := os.Open(fileLoc)
	if err != nil {
		return nil, errors.Wrapf(err, "opening file '%s'", fn)
	}
	defer f.Close()

	var completions []virtualTaskCompletionFile
	if err := utility.ReadJSON(f, &completions); err != nil {
		return nil, errors.Wrapf(err, "reading JSON from file '%s'", fn)
	}
	return completions, nil
}
