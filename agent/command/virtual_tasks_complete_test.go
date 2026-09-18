package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompleteVirtualTasksParseParams(t *testing.T) {
	t.Run("NoFilesShouldError", func(t *testing.T) {
		cmd := &completeVirtualTasks{}
		require.Error(t, cmd.ParseParams(map[string]any{}))
	})
	t.Run("ValidParamsDecodeCorrectly", func(t *testing.T) {
		cmd := &completeVirtualTasks{}
		require.NoError(t, cmd.ParseParams(map[string]any{
			"files":    []string{"results.json"},
			"optional": true,
		}))
		assert.Equal(t, []string{"results.json"}, cmd.Files)
		assert.True(t, cmd.Optional)
	})
}

func TestCompleteVirtualTasksExecute(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FileNotFoundShouldError": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd := &completeVirtualTasks{Files: []string{"nonexistent.json"}}
			assert.Error(t, cmd.Execute(ctx, comm, logger, conf))
		},
		"OptionalNoFilesSucceeds": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd := &completeVirtualTasks{Files: []string{"nonexistent_*.json"}, Optional: true}
			assert.NoError(t, cmd.Execute(ctx, comm, logger, conf))
		},
		"SingleFileSuccess": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			completions := []apimodels.VirtualTaskCompletion{
				{TaskID: "task1", Execution: 0, Status: "succeeded"},
				{TaskID: "task2", Execution: 0, Status: "failed"},
			}
			path := filepath.Join(conf.WorkDir, "results.json")
			require.NoError(t, utility.WriteJSONFile(path, completions))

			cmd := &completeVirtualTasks{Files: []string{"results.json"}}
			require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

			assert.Len(t, comm.CompleteVirtualTasksCompletions, 2)
			assert.Equal(t, "task1", comm.CompleteVirtualTasksCompletions[0].TaskID)
			assert.Equal(t, "task2", comm.CompleteVirtualTasksCompletions[1].TaskID)
		},
		"MultipleFilesAggregated": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			require.NoError(t, utility.WriteJSONFile(
				filepath.Join(conf.WorkDir, "results1.json"),
				[]apimodels.VirtualTaskCompletion{{TaskID: "task1", Execution: 0, Status: "succeeded"}},
			))
			require.NoError(t, utility.WriteJSONFile(
				filepath.Join(conf.WorkDir, "results2.json"),
				[]apimodels.VirtualTaskCompletion{{TaskID: "task2", Execution: 0, Status: "succeeded"}},
			))

			cmd := &completeVirtualTasks{Files: []string{"results1.json", "results2.json"}}
			require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

			assert.Len(t, comm.CompleteVirtualTasksCompletions, 2)
		},
		"BatchesOver100": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			completions := make([]apimodels.VirtualTaskCompletion, 150)
			for i := range completions {
				completions[i] = apimodels.VirtualTaskCompletion{TaskID: "task", Execution: 0, Status: "succeeded"}
			}
			require.NoError(t, utility.WriteJSONFile(filepath.Join(conf.WorkDir, "results.json"), completions))

			cmd := &completeVirtualTasks{Files: []string{"results.json"}}
			require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

			assert.Len(t, comm.CompleteVirtualTasksCompletions, 150)
		},
		"APIFailureReturnsError": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			require.NoError(t, utility.WriteJSONFile(
				filepath.Join(conf.WorkDir, "results.json"),
				[]apimodels.VirtualTaskCompletion{{TaskID: "task1", Execution: 0, Status: "succeeded"}},
			))
			comm.CompleteVirtualTasksShouldFail = true

			cmd := &completeVirtualTasks{Files: []string{"results.json"}}
			assert.Error(t, cmd.Execute(ctx, comm, logger, conf))
		},
		"PartialTaskFailureReturnsError": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			require.NoError(t, utility.WriteJSONFile(
				filepath.Join(conf.WorkDir, "results.json"),
				[]apimodels.VirtualTaskCompletion{
					{TaskID: "task1", Execution: 0, Status: "succeeded"},
					{TaskID: "task2", Execution: 0, Status: "succeeded"},
				},
			))
			comm.CompleteVirtualTasksResponse = &apimodels.CompleteVirtualTasksResponse{
				Results: []apimodels.VirtualTaskCompletionResult{
					{TaskID: "task1", Outcome: apimodels.VirtualTaskCompletionOutcomeSuccess},
					{TaskID: "task2", Outcome: apimodels.VirtualTaskCompletionOutcomeFailed, Reason: "task is not a virtual task"},
				},
			}

			cmd := &completeVirtualTasks{Files: []string{"results.json"}}
			err := cmd.Execute(ctx, comm, logger, conf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "task2")
		},
		"InvalidJSONShouldError": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			path := filepath.Join(conf.WorkDir, "bad.json")
			require.NoError(t, utility.WriteFile(path, "not valid json"))

			cmd := &completeVirtualTasks{Files: []string{"bad.json"}}
			assert.Error(t, cmd.Execute(ctx, comm, logger, conf))
		},
		"EmptyCompletionsArraySucceeds": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			require.NoError(t, utility.WriteJSONFile(
				filepath.Join(conf.WorkDir, "empty.json"),
				[]apimodels.VirtualTaskCompletion{},
			))

			cmd := &completeVirtualTasks{Files: []string{"empty.json"}}
			require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

			assert.Empty(t, comm.CompleteVirtualTasksCompletions)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()
			tmpDir := t.TempDir()

			conf := &internal.TaskConfig{
				Task: task.Task{
					Id:     "mock_id",
					Secret: "mock_secret",
				},
				BuildVariant: model.BuildVariant{
					Name: "build_variant",
				},
				Expansions: util.Expansions{},
				WorkDir:    tmpDir,
			}

			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			tCase(ctx, t, comm, logger, conf)
		})
	}
}
