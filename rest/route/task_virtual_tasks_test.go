package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestCompleteVirtualTasks(t *testing.T) {
	const (
		projectID     = "virtual_project"
		versionID     = "virtual_version"
		buildID       = "virtual_build"
		runnerTaskID  = "runner_task"
		virtualTaskID = "virtual_task"
		distroID      = "virtual_distro"
	)

	successfulCompletion := func() apimodels.VirtualTaskCompletion {
		return apimodels.VirtualTaskCompletion{
			TaskID: virtualTaskID,
			Status: evergreen.TaskSucceeded,
		}
	}
	requireResults := func(t *testing.T, resp gimlet.Responder, numResults int) []apimodels.VirtualTaskCompletionResult {
		require.NotNil(t, resp)
		require.Equal(t, http.StatusCreated, resp.Status())
		data, ok := resp.Data().(apimodels.CompleteVirtualTasksResponse)
		require.True(t, ok)
		require.Len(t, data.Results, numResults)
		return data.Results
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *completeVirtualTasksHandler, env evergreen.Environment){
		"PushCompletesVirtualTaskWithResultsAndArtifacts": func(ctx context.Context, t *testing.T, h *completeVirtualTasksHandler, env evergreen.Environment) {
			createdAt := time.Now().UTC().Round(time.Millisecond)
			h.body = apimodels.CompleteVirtualTasksRequest{Tasks: []apimodels.VirtualTaskCompletion{
				{
					TaskID: virtualTaskID,
					Status: evergreen.TaskSucceeded,
					TestResults: &apimodels.VirtualTaskTestResults{
						Stats:        testresult.TaskTestResultsStats{TotalCount: 3, FailedCount: 1},
						FailedSample: []string{"failed_test"},
						CreatedAt:    createdAt,
					},
					Artifacts: []apimodels.VirtualTaskArtifact{
						{Name: "test.log", URL: "https://example.com/test.log", Visibility: artifact.Public},
					},
					ExternalMetadata: &apimodels.ExternalExecutionMetadata{
						EngFlowInvocationID: "inv-xyz",
						ShardID:             "shard-3",
					},
				},
			}}

			results := requireResults(t, h.Run(ctx), 1)
			assert.Equal(t, apimodels.VirtualTaskCompletionOutcomeSuccess, results[0].Outcome)
			assert.Empty(t, results[0].Reason)

			vt, err := task.FindOneId(ctx, virtualTaskID)
			require.NoError(t, err)
			require.NotNil(t, vt)
			assert.Equal(t, evergreen.TaskSucceeded, vt.Status)
			assert.Equal(t, evergreen.TaskSucceeded, vt.Details.Status)
			assert.Empty(t, vt.Details.ExecutionPlatform, "a push-completed task should not have its own execution platform since it was never run")
			assert.Equal(t, runnerTaskID, vt.CompletedBy)
			assert.True(t, vt.StartTime.Equal(vt.FinishTime), "a push-completed task should have no duration of its own")
			assert.Zero(t, vt.TimeTaken)
			assert.Empty(t, vt.ActivatedBy)
			assert.Empty(t, vt.HostId)
			require.NotNil(t, vt.Details.ExternalExecutionMetadata)
			assert.Equal(t, "inv-xyz", vt.Details.ExternalExecutionMetadata.EngFlowInvocationID)
			assert.Equal(t, "shard-3", vt.Details.ExternalExecutionMetadata.ShardID)
			assert.NotNil(t, vt.TaskOutputInfo, "the task output info should be set so pushed results can be located")
			assert.True(t, vt.HasTestResults)
			assert.True(t, vt.ResultsFailed)

			var record testresult.DbTaskTestResults
			require.NoError(t, env.CedarDB().Collection(testresult.Collection).FindOne(ctx, task.ByTaskIDAndExecution(virtualTaskID, 0)).Decode(&record))
			assert.Equal(t, 3, record.Stats.TotalCount)
			assert.Equal(t, 1, record.Stats.FailedCount)
			assert.Equal(t, []string{"failed_test"}, record.FailedTestsSample)
			assert.True(t, createdAt.Equal(record.CreatedAt))

			entry, err := artifact.FindOne(ctx, artifact.ByTaskIdAndExecution(virtualTaskID, 0))
			require.NoError(t, err)
			require.NotNil(t, entry)
			require.Len(t, entry.Files, 1)
			assert.Equal(t, "test.log", entry.Files[0].Name)
			assert.Equal(t, "https://example.com/test.log", entry.Files[0].Link)
		},
		"PushCompletesVirtualTaskAsFailed": func(ctx context.Context, t *testing.T, h *completeVirtualTasksHandler, env evergreen.Environment) {
			h.body = apimodels.CompleteVirtualTasksRequest{Tasks: []apimodels.VirtualTaskCompletion{
				{TaskID: virtualTaskID, Status: evergreen.TaskFailed},
			}}

			results := requireResults(t, h.Run(ctx), 1)
			assert.Equal(t, apimodels.VirtualTaskCompletionOutcomeSuccess, results[0].Outcome)

			vt, err := task.FindOneId(ctx, virtualTaskID)
			require.NoError(t, err)
			require.NotNil(t, vt)
			assert.Equal(t, evergreen.TaskFailed, vt.Status)
			assert.Equal(t, evergreen.TaskFailed, vt.GetDisplayStatus())
			assert.Equal(t, runnerTaskID, vt.CompletedBy)
		},
		"RunningTaskNoOps": func(ctx context.Context, t *testing.T, h *completeVirtualTasksHandler, env evergreen.Environment) {
			require.NoError(t, task.UpdateOne(ctx, task.ById(virtualTaskID), bson.M{
				"$set": bson.M{task.StatusKey: evergreen.TaskStarted, task.ActivatedKey: true},
			}))
			h.body = apimodels.CompleteVirtualTasksRequest{Tasks: []apimodels.VirtualTaskCompletion{successfulCompletion()}}

			results := requireResults(t, h.Run(ctx), 1)
			assert.Equal(t, apimodels.VirtualTaskCompletionOutcomeSuccess, results[0].Outcome)
			assert.Contains(t, results[0].Reason, "already running")

			vt, err := task.FindOneId(ctx, virtualTaskID)
			require.NoError(t, err)
			require.NotNil(t, vt)
			assert.Equal(t, evergreen.TaskStarted, vt.Status)
			assert.Empty(t, vt.CompletedBy)
		},
		"ActivatedUndispatchedTaskIsDequeuedAndCompleted": func(ctx context.Context, t *testing.T, h *completeVirtualTasksHandler, env evergreen.Environment) {
			require.NoError(t, task.UpdateOne(ctx, task.ById(virtualTaskID), bson.M{
				"$set": bson.M{task.ActivatedKey: true},
			}))
			queue := model.NewTaskQueue(distroID, []model.TaskQueueItem{{Id: virtualTaskID}}, model.DistroQueueInfo{})
			require.NoError(t, queue.Save(ctx))
			h.body = apimodels.CompleteVirtualTasksRequest{Tasks: []apimodels.VirtualTaskCompletion{successfulCompletion()}}

			results := requireResults(t, h.Run(ctx), 1)
			assert.Equal(t, apimodels.VirtualTaskCompletionOutcomeSuccess, results[0].Outcome)

			vt, err := task.FindOneId(ctx, virtualTaskID)
			require.NoError(t, err)
			require.NotNil(t, vt)
			assert.Equal(t, evergreen.TaskSucceeded, vt.Status)

			dbQueue, err := model.LoadTaskQueue(ctx, distroID)
			require.NoError(t, err)
			assert.Zero(t, dbQueue.Length())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()

			colls := []string{task.Collection, build.Collection, model.VersionCollection, model.ParserProjectCollection, model.ProjectRefCollection, model.TaskQueuesCollection, artifact.Collection, event.EventCollection, evergreen.ScopeCollection, evergreen.RoleCollection}
			require.NoError(t, db.ClearCollections(colls...))
			t.Cleanup(func() {
				assert.NoError(t, db.ClearCollections(colls...))
			})

			env := testutil.NewEnvironment(ctx, t)
			require.NoError(t, task.ClearTestResults(ctx, env))
			t.Cleanup(func() {
				assert.NoError(t, task.ClearTestResults(context.Background(), env))
			})

			require.NoError(t, evergreen.SetServiceFlags(ctx, evergreen.ServiceFlags{}))

			pRef := model.ProjectRef{
				Id:                  projectID,
				Identifier:          "virtual-project",
				Enabled:             true,
				VirtualTasksEnabled: utility.TruePtr(),
			}
			require.NoError(t, pRef.Insert(ctx))
			parserProj := model.ParserProject{Id: versionID}
			require.NoError(t, parserProj.Insert(ctx))
			testVersion := model.Version{Id: versionID, Branch: projectID}
			require.NoError(t, testVersion.Insert(ctx))
			testBuild := build.Build{Id: buildID, Project: projectID, Version: versionID}
			require.NoError(t, testBuild.Insert(ctx))

			runnerTask := task.Task{
				Id:        runnerTaskID,
				Status:    evergreen.TaskStarted,
				Activated: true,
				HostId:    "runner_host",
				Project:   projectID,
				BuildId:   buildID,
				Version:   versionID,
				Requester: evergreen.PatchVersionRequester,
			}
			require.NoError(t, runnerTask.Insert(ctx))
			virtualTask := task.Task{
				Id:           virtualTaskID,
				DisplayName:  "virtual_task_display_name",
				Status:       evergreen.TaskUndispatched,
				Activated:    false,
				IsVirtual:    true,
				Project:      projectID,
				BuildVariant: "bv",
				BuildId:      buildID,
				Version:      versionID,
				DistroId:     distroID,
				CreateTime:   time.Now(),
				Requester:    evergreen.PatchVersionRequester,
			}
			require.NoError(t, virtualTask.Insert(ctx))

			h, ok := makeCompleteVirtualTasks(env).(*completeVirtualTasksHandler)
			require.True(t, ok)
			h.taskID = runnerTaskID
			ctx = context.WithValue(ctx, model.ApiTaskKey, &runnerTask)
			tCase(ctx, t, h, env)
		})
	}
}
