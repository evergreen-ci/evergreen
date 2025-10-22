package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	taskSecret = "tasksecret"
)

func TestAgentGetExpansionsAndVars(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*getExpansionsAndVarsHandler)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/task/t1/fetch_vars", nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "t1"})
			req.Header.Add(evergreen.HostHeader, "host_id")
			assert.NoError(t, rh.Parse(ctx, req))
			assert.Equal(t, "t1", rh.taskID)
			assert.Equal(t, "host_id", rh.hostID)
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			rh.taskID = "t2"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionsAndVars)
			require.True(t, ok)
			assert.Equal(t, rh.taskID, data.Expansions.Get("task_id"))
			assert.Equal(t, map[string]string{"a": "1", "b": "3"}, data.Vars)
			assert.Equal(t, map[string]bool{"b": true}, data.PrivateVars)
			assert.Equal(t, []string{"pass", "secret"}, data.RedactKeys)
		},
		"RunSucceedsWithParamsSetOnVersion": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			rh.taskID = "t1"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionsAndVars)
			require.True(t, ok)
			assert.Equal(t, map[string]string{"a": "1", "b": "3"}, data.Vars)
			assert.Equal(t, map[string]string{"a": "4"}, data.Parameters)
			assert.Equal(t, map[string]bool{"b": true}, data.PrivateVars)
			assert.Equal(t, []string{"pass", "secret"}, data.RedactKeys)
		},
		"RunSucceedsWithHostDistroExpansions": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			rh.taskID = "t1"
			rh.hostID = "host_id"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionsAndVars)
			require.True(t, ok)
			assert.Equal(t, rh.taskID, data.Expansions.Get("task_id"))
			assert.Equal(t, "distro_expansion_value", data.Expansions.Get("distro_expansion_key"))
			assert.Equal(t, "password", data.InternalRedactions[hostServicePasswordPlaceholder])
			assert.Equal(t, map[string]string{"a": "1", "b": "3"}, data.Vars)
			assert.Equal(t, map[string]string{"a": "4"}, data.Parameters)
			assert.Equal(t, map[string]bool{"b": true}, data.PrivateVars)
			assert.Equal(t, []string{"pass", "secret"}, data.RedactKeys)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.Settings().LoggerConfig.RedactKeys = []string{"pass", "secret"}

			testutil.ConfigureIntegrationTest(t, env.Settings())

			require.NoError(t, db.ClearCollections(host.Collection, task.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, model.VersionCollection, model.ParserProjectCollection))

			const hostID = "host_id"
			t1 := task.Task{
				Id:      "t1",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334455",
				HostId:  hostID,
			}
			t2 := task.Task{
				Id:      "t2",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334456",
			}
			pRef := model.ProjectRef{
				Id:    "p1",
				Owner: "evergreen-ci",
				Repo:  "sample",
			}
			vars := &model.ProjectVars{
				Id:          "p1",
				Vars:        map[string]string{"a": "1", "b": "3"},
				PrivateVars: map[string]bool{"b": true},
			}
			v1 := &model.Version{
				Id:         "aaaaaaaaaaff001122334455",
				Revision:   "1234",
				Requester:  evergreen.GitTagRequester,
				CreateTime: time.Now(),
				Parameters: []patch.Parameter{
					{
						Key:   "a",
						Value: "4",
					},
				},
			}
			v2 := &model.Version{
				Id:         "aaaaaaaaaaff001122334456",
				Revision:   "1234",
				Requester:  evergreen.GitTagRequester,
				CreateTime: time.Now(),
			}
			pp1 := model.ParserProject{
				Id:         "aaaaaaaaaaff001122334455",
				Parameters: []model.ParameterInfo{},
			}
			pp2 := model.ParserProject{
				Id:         "aaaaaaaaaaff001122334456",
				Parameters: []model.ParameterInfo{},
			}
			h := host.Host{
				Id:                   hostID,
				RunningTask:          t1.Id,
				RunningTaskExecution: t1.Execution,
				Distro: distro.Distro{
					Arch: "windows",
					Expansions: []distro.Expansion{
						{
							Key:   "distro_expansion_key",
							Value: "distro_expansion_value",
						},
					},
				},
				ServicePassword: "password",
			}
			require.NoError(t, t1.Insert(t.Context()))
			require.NoError(t, t2.Insert(t.Context()))
			require.NoError(t, pRef.Insert(t.Context()))
			require.NoError(t, vars.Insert(t.Context()))
			require.NoError(t, v1.Insert(t.Context()))
			require.NoError(t, v2.Insert(t.Context()))
			require.NoError(t, pp1.Insert(t.Context()))
			require.NoError(t, pp2.Insert(t.Context()))
			require.NoError(t, h.Insert(ctx))

			r, ok := makeGetExpansionsAndVars(env.Settings()).(*getExpansionsAndVarsHandler)
			require.True(t, ok)

			tCase(ctx, t, r)
		})
	}
}

func TestMarkTaskForReset(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*markTaskForRestartHandler)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/agent/task/t1/reset", nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "t1"})
			assert.NoError(t, rh.Parse(ctx, req))
			assert.Equal(t, "t1", rh.taskID)
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			rh.taskID = "t2"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err := task.FindOneId(ctx, "t2")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.True(t, foundTask.ResetWhenFinished)
			assert.True(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)

			// Should fail if the task resets and tries to auto restart again
			require.NoError(t, foundTask.MarkEnd(ctx, time.Now(), &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}))
			require.NoError(t, foundTask.Archive(ctx))
			require.NoError(t, foundTask.Reset(ctx, ""))
			resp = rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusBadRequest, resp.Status())

			foundTask, err = task.FindOneId(ctx, "t2")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.False(t, foundTask.ResetWhenFinished)
			assert.False(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, evergreen.TaskUndispatched, foundTask.Status)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)
		},
		"RunSucceedsWithDisplayTask": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			rh.taskID = "et1"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err := task.FindOneId(ctx, "dt")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.True(t, foundTask.ResetWhenFinished)
			assert.True(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)

			// Should not error if another execution task tries to mark the display task for restart
			// before the display task has finished
			rh.taskID = "et2"
			resp = rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err = task.FindOneId(ctx, "dt")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.True(t, foundTask.ResetWhenFinished)
			assert.True(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)
		},
		"SuccessfullyChecksMaxRestartLimit": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			// Should succeed normally for first task
			rh.taskID = "t2"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err := task.FindOneId(ctx, "t2")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.True(t, foundTask.ResetWhenFinished)
			assert.True(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)

			// Should fail on second task since a limit is in place of 1
			rh.taskID = "t3"
			resp = rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusInternalServerError, resp.Status())
			require.NotNil(t, resp.Data())
			assert.Contains(t, resp.Data().(gimlet.ErrorResponse).Message, "project 'p1' has auto-restarted 1 out of 1 allowed tasks in the past day")

			// Should succeed again if simulating >1 day passing
			pRef := model.ProjectRef{
				Id:                      "p1",
				NumAutoRestartedTasks:   1,
				LastAutoRestartedTaskAt: time.Now().Add(-25 * time.Hour),
			}
			require.NoError(t, pRef.Replace(t.Context()))
			rh.taskID = "t4"
			resp = rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err = task.FindOneId(ctx, "t4")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.True(t, foundTask.ResetWhenFinished)
			assert.True(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection))
			settings := &evergreen.Settings{
				TaskLimits: evergreen.TaskLimitsConfig{
					MaxDailyAutomaticRestarts: 1,
				},
			}
			require.NoError(t, evergreen.UpdateConfig(ctx, settings))

			et1 := task.Task{
				Id:      "et1",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334455",
			}
			et2 := task.Task{
				Id:      "et2",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334455",
			}
			dt := task.Task{
				Id:             "dt",
				Project:        "p1",
				Version:        "aaaaaaaaaaff001122334455",
				DisplayOnly:    true,
				ExecutionTasks: []string{"et1", "et2"},
			}
			t2 := task.Task{
				Id:      "t2",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334456",
			}
			t3 := task.Task{
				Id:      "t3",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334456",
			}
			t4 := task.Task{
				Id:      "t4",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334456",
			}
			pRef := model.ProjectRef{
				Id: "p1",
			}
			require.NoError(t, pRef.Insert(t.Context()))
			require.NoError(t, et1.Insert(t.Context()))
			require.NoError(t, et2.Insert(t.Context()))
			require.NoError(t, dt.Insert(t.Context()))
			require.NoError(t, t2.Insert(t.Context()))
			require.NoError(t, t3.Insert(t.Context()))
			require.NoError(t, t4.Insert(t.Context()))
			r, ok := makeMarkTaskForRestart().(*markTaskForRestartHandler)
			require.True(t, ok)

			tCase(ctx, t, r)
		})
	}
}

func TestAgentSetup(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentSetup, s *evergreen.Settings){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentSetup, s *evergreen.Settings) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentSetup)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentSetup, s *evergreen.Settings) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/setup", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *agentSetup, s *evergreen.Settings) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.AgentSetupData)
			require.True(t, ok)
			assert.Equal(t, data.SplunkServerURL, s.Splunk.SplunkConnectionInfo.ServerURL)
			assert.Equal(t, data.SplunkClientToken, s.Splunk.SplunkConnectionInfo.Token)
			assert.Equal(t, data.SplunkChannel, s.Splunk.SplunkConnectionInfo.Channel)
			assert.Equal(t, data.MaxExecTimeoutSecs, s.TaskLimits.MaxExecTimeoutSecs)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *agentSetup, s *evergreen.Settings) {
			*s = evergreen.Settings{}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.AgentSetupData)
			require.True(t, ok)
			assert.Zero(t, data)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			s := &evergreen.Settings{
				Splunk: evergreen.SplunkConfig{
					SplunkConnectionInfo: send.SplunkConnectionInfo{
						ServerURL: "server_url",
						Token:     "token",
						Channel:   "channel",
					},
				},
				Providers: evergreen.CloudProviders{
					AWS: evergreen.AWSConfig{
						EC2Keys: []evergreen.EC2Key{
							{
								Name:   "ec2-key",
								Key:    "key",
								Secret: "secret",
							},
						},
					},
				},
				TaskLimits: evergreen.TaskLimitsConfig{
					MaxExecTimeoutSecs: 10,
				},
			}

			r, ok := makeAgentSetup(s).(*agentSetup)
			require.True(t, ok)

			tCase(ctx, t, r, s)
		})
	}
}

func TestDownstreamParams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(patch.Collection, task.Collection, host.Collection))
	parameters := []patch.Parameter{
		{Key: "key_1", Value: "value_1"},
		{Key: "key_2", Value: "value_2"},
	}

	versionId := "myTestVersion"
	parentPatchId := "5bedc62ee4055d31f0340b1d"
	parentPatch := patch.Patch{
		Id:      mgobson.ObjectIdHex(parentPatchId),
		Version: versionId,
	}
	require.NoError(t, parentPatch.Insert(t.Context()))

	hostId := "h1"
	projectId := "proj"
	buildID := "b1"

	task1 := task.Task{
		Id:        "task1",
		Status:    evergreen.TaskStarted,
		Activated: true,
		HostId:    hostId,
		Secret:    taskSecret,
		Project:   projectId,
		BuildId:   buildID,
		Version:   versionId,
	}
	require.NoError(t, task1.Insert(t.Context()))

	sampleHost := host.Host{
		Id: hostId,
		Distro: distro.Distro{
			Provider: evergreen.ProviderNameEc2Fleet,
		},
		Secret:                hostSecret,
		RunningTask:           task1.Id,
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		AgentRevision:         evergreen.AgentVersion,
		LastTaskCompletedTime: time.Now().Add(-20 * time.Minute).Round(time.Second),
	}
	require.NoError(t, sampleHost.Insert(ctx))

	q := queue.NewLocalLimitedSize(4, 2048)
	require.NoError(t, q.Start(ctx))

	r, ok := makeSetDownstreamParams().(*setDownstreamParamsHandler)
	r.taskID = "task1"
	r.downstreamParams = parameters
	require.True(t, ok)

	resp := r.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	p, err := patch.FindOneId(t.Context(), parentPatchId)
	require.NoError(t, err)
	assert.Equal(t, p.Triggers.DownstreamParameters[0].Key, parameters[0].Key)
	assert.Equal(t, p.Triggers.DownstreamParameters[0].Value, parameters[0].Value)
	assert.Equal(t, p.Triggers.DownstreamParameters[1].Key, parameters[1].Key)
	assert.Equal(t, p.Triggers.DownstreamParameters[1].Value, parameters[1].Value)
}

func TestAgentGetProjectRef(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection))
	}()

	task1 := &task.Task{
		Id:      "task1",
		Project: "project1",
	}
	projRef1 := &model.ProjectRef{Id: "project1"}
	require.NoError(t, task1.Insert(t.Context()))
	require.NoError(t, projRef1.Insert(t.Context()))

	task2 := &task.Task{
		Id:      "task2",
		Project: "project2",
	}
	require.NoError(t, task2.Insert(t.Context()))

	for _, test := range []struct {
		name           string
		taskID         string
		expectedStatus int
		expectedData   *model.ProjectRef
	}{
		{
			name:           "TaskDNE",
			taskID:         "DNE",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "ProjectRefDNE",
			taskID:         task2.Id,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "ProjectRef",
			taskID:         task1.Id,
			expectedStatus: http.StatusOK,
			expectedData:   projRef1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := &getProjectRefHandler{taskID: test.taskID}

			resp := h.Run(ctx)
			assert.Equal(t, test.expectedStatus, resp.Status())
			if test.expectedData != nil {
				assert.EqualValues(t, test.expectedData, resp.Data())
			}
		})
	}
}

func TestCreateInstallationToken(t *testing.T) {
	validOwner := "owner"
	validRepo := "repo"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *createInstallationTokenForClone){
		"ParseErrorsOnEmptyOwnerAndRepo": func(ctx context.Context, t *testing.T, handler *createInstallationTokenForClone) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", "", "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(ctx, request))
		},
		"ParseErrorsOnEmptyOwner": func(ctx context.Context, t *testing.T, handler *createInstallationTokenForClone) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", "", validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing owner")
		},
		"ParseErrorsOnEmptyRepo": func(ctx context.Context, t *testing.T, handler *createInstallationTokenForClone) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", validOwner, "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing repo")
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *createInstallationTokenForClone) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", validOwner, validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.NoError(t, handler.Parse(ctx, request))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			r, ok := makeCreateInstallationToken(env).(*createInstallationTokenForClone)
			require.True(t, ok)

			tCase(ctx, t, r)
		})
	}
}

func TestUpsertCheckRunParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, patch.Collection))

	versionId := "aaaaaaaaaaff001122334455"
	patch := patch.Patch{
		Id:      patch.NewId(versionId),
		Version: versionId,
	}
	require.NoError(t, patch.Insert(t.Context()))

	task1 := task.Task{
		Id:        "task1",
		Status:    evergreen.TaskStarted,
		Activated: true,
		HostId:    "h1",
		Secret:    taskSecret,
		Project:   "proj",
		BuildId:   "b1",
		Version:   versionId,
		Requester: evergreen.GithubPRRequester,
	}
	require.NoError(t, task1.Insert(t.Context()))

	r, ok := makeCheckRun(&evergreen.Settings{}).(*checkRunHandler)
	require.True(t, ok)
	jsonCheckrun := `
	{
	        "title": "This is my report",
	        "summary": "We found 6 failures and 2 warnings",
	        "text": "It looks like there are some errors on lines 2 and 4.",
	        "annotations": [
	            {
	                "path": "README.md",
	                "annotation_level": "warning",
	                "title": "Error Detector",
	                "message": "message",
	                "raw_details": "Do you mean this other thing?",
	                "start_line": 2,
	                "end_line": 4
	            }
	        ]
	}
	`
	gh := github.CheckRunOutput{}
	assert.NoError(t, json.Unmarshal([]byte(jsonCheckrun), &gh))

	request, err := http.NewRequest(http.MethodPost, "/task/{task_id}/check_run", bytes.NewReader([]byte(jsonCheckrun)))
	assert.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"task_id": "task1"})

	assert.NoError(t, r.Parse(ctx, request))
	assert.Equal(t, "task1", r.taskID)

	assert.Equal(t, "This is my report", utility.FromStringPtr(r.checkRunOutput.Title))
	assert.Len(t, r.checkRunOutput.Annotations, 1)
}

func TestCreateGitHubDynamicAccessToken(t *testing.T) {
	route := "/task/%s/github_dynamic_access_token/%s/%s"
	owner := "owner"
	repo := "repo"
	taskID := "taskID"
	json := []byte(`{ "checks": "read", "actions": "write" }`)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *createGitHubDynamicAccessToken){
		"ParseErrorsOnEmptyOwner": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID, "", repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": "", "repo": repo, "task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing owner")
		},
		"ParseErrorsOnEmptyRepo": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID, owner, "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": "", "task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing repo")
		},
		"ParseErrorsOnEmptyTaskID": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, "", owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "task_id": ""}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing task_id")
		},
		"ParseSucceedsOnNullBody": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID, owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader([]byte("null")))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			require.NoError(t, handler.Parse(ctx, request))
			require.True(t, handler.allPermissions)
		},
		"ParseSucceedsOnEmptyBody": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID, owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader([]byte("{}")))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			require.NoError(t, handler.Parse(ctx, request))
			require.True(t, handler.allPermissions)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID, owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			require.NoError(t, handler.Parse(ctx, request))
			require.NotNil(t, handler.permissions)
			require.False(t, handler.allPermissions)
			assert.Equal(t, "read", utility.FromStringPtr(handler.permissions.Checks))
			assert.Equal(t, "write", utility.FromStringPtr(handler.permissions.Actions))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			r, ok := makeCreateGitHubDynamicAccessToken(env).(*createGitHubDynamicAccessToken)
			require.True(t, ok)

			tCase(ctx, t, r)
		})
	}
}

func TestRevokeGitHubDynamicAccessToken(t *testing.T) {
	const (
		route  = "/task/%s/github_dynamic_access_token"
		taskID = "taskID"
		token  = "some-token-would-be-here"
	)
	body, err := json.Marshal(apimodels.Token{Token: token})
	require.NoError(t, err)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *revokeGitHubDynamicAccessToken){
		"ParseErrorsOnEmptyTaskID": func(ctx context.Context, t *testing.T, handler *revokeGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, "")
			request, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(body))
			require.NoError(t, err)

			options := map[string]string{"task_id": ""}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing task_id")
		},
		"ParseErrorsOnNilBody": func(ctx context.Context, t *testing.T, handler *revokeGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID)
			request, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(nil))
			require.NoError(t, err)

			options := map[string]string{"task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "reading token JSON request body for task 'taskID'")
		}, "ParseErrorsOnEmptyBody": func(ctx context.Context, t *testing.T, handler *revokeGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID)
			request, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader([]byte("{}")))
			require.NoError(t, err)

			options := map[string]string{"task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing token")
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *revokeGitHubDynamicAccessToken) {
			url := fmt.Sprintf(route, taskID)
			request, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(body))
			require.NoError(t, err)

			options := map[string]string{"task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			require.NoError(t, handler.Parse(ctx, request))
			assert.Equal(t, token, handler.body.Token)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			r, ok := makeRevokeGitHubDynamicAccessToken(env).(*revokeGitHubDynamicAccessToken)
			require.True(t, ok)

			tCase(ctx, t, r)
		})
	}
}

func TestAWSAssumeRole(t *testing.T) {
	route := "/task/%s/aws/assume_role"
	taskID := "taskID"
	projectID := "project_id"
	repoRefID := "repo_ref_id"
	roleARN := "unique_role_arn"
	policy := "policy-num"
	var duration int32 = 1600
	json := `{"role_arn": "%s", "policy": "%s", "duration_seconds": %d}`

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, ar *awsAssumeRole){
		"ParseErrorsOnNilBody": func(ctx context.Context, t *testing.T, handler *awsAssumeRole) {
			url := fmt.Sprintf(route, taskID)
			request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(nil))
			require.NoError(t, err)

			options := map[string]string{"task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "reading assume role body for task 'taskID'")
		},
		"ParseErrorsOnEmptyBody": func(ctx context.Context, t *testing.T, handler *awsAssumeRole) {
			url := fmt.Sprintf(route, taskID)
			request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
			require.NoError(t, err)

			options := map[string]string{"task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "validating assume role body for task 'taskID'")
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *awsAssumeRole) {
			url := fmt.Sprintf(route, taskID)
			request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(fmt.Sprintf(json, roleARN, policy, duration))))
			require.NoError(t, err)

			options := map[string]string{"task_id": taskID}
			request = gimlet.SetURLVars(request, options)

			require.NoError(t, handler.Parse(ctx, request))
			assert.Equal(t, taskID, handler.taskID)
			assert.Equal(t, roleARN, handler.body.RoleARN)
			assert.Equal(t, policy, utility.FromStringPtr(handler.body.Policy))
			assert.Equal(t, duration, utility.FromInt32Ptr(handler.body.DurationSeconds))

			t.Run("RunErrorsOnNilTask", func(t *testing.T) {
				resp := handler.Run(ctx)
				require.NotNil(t, resp)
				require.Equal(t, http.StatusBadRequest, resp.Status(), resp.Data())
			})

			t.Run("RunSucceeds", func(t *testing.T) {
				task := task.Task{Id: taskID, Project: projectID, Requester: "requester"}
				require.NoError(t, task.Insert(t.Context()))
				project := model.ProjectRef{Id: projectID, RepoRefId: repoRefID}
				require.NoError(t, project.Insert(t.Context()))
				repoRef := model.RepoRef{ProjectRef: model.ProjectRef{Id: repoRefID}}
				require.NoError(t, repoRef.Replace(t.Context()))

				resp := handler.Run(ctx)
				require.NotNil(t, resp)
				require.Equal(t, http.StatusOK, resp.Status(), resp.Data())
				data, ok := resp.Data().(apimodels.AWSCredentials)
				require.True(t, ok)
				assert.NotEmpty(t, data.AccessKeyID)
				assert.NotEmpty(t, data.SecretAccessKey)
				assert.NotEmpty(t, data.SessionToken)
				assert.NotEmpty(t, data.Expiration)
				assert.NotEmpty(t, data.ExternalID)
			})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, model.RepoRefAdminsKey))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(t.Context()))

			manager := cloud.GetSTSManager(true)

			r, ok := makeAWSAssumeRole(manager).(*awsAssumeRole)
			require.True(t, ok)

			tCase(t.Context(), t, r)
		})
	}

}

func TestStartTaskWithOtelMetadata(t *testing.T) {
	testCases := []struct {
		name          string
		diskDevices   []string
		traceID       string
		expectedDisk  []string
		expectedTrace string
	}{
		{
			name:          "BothDiskDevicesAndTraceID",
			diskDevices:   []string{"sda1", "sdb1"},
			traceID:       "test-trace-id-123",
			expectedDisk:  []string{"sda1", "sdb1"},
			expectedTrace: "test-trace-id-123",
		},
		{
			name:          "JustTraceID",
			diskDevices:   nil,
			traceID:       "test-trace-id-456",
			expectedDisk:  nil,
			expectedTrace: "test-trace-id-456",
		},
		{
			name:          "JustDiskDevices",
			diskDevices:   []string{"nvme0n1", "nvme1n1"},
			traceID:       "",
			expectedDisk:  []string{"nvme0n1", "nvme1n1"},
			expectedTrace: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(task.Collection, host.Collection))

			// Create a test task
			testTask := task.Task{
				Id:      "test-task-id",
				Project: "test-project",
				Version: "test-version",
				Status:  evergreen.TaskUndispatched,
			}
			require.NoError(t, testTask.Insert(ctx))

			// Create a test host
			testHost := host.Host{
				Id:          "test-host-id",
				Status:      evergreen.HostRunning,
				RunningTask: testTask.Id,
			}
			require.NoError(t, testHost.Insert(ctx))

			// Create the request body using TaskStartRequest
			requestBody := apimodels.TaskStartRequest{
				TraceID:     tc.traceID,
				DiskDevices: tc.diskDevices,
			}
			bodyBytes, err := json.Marshal(requestBody)
			require.NoError(t, err)

			// Create the HTTP request
			req, err := http.NewRequest(http.MethodPost, "/task/test-task-id/start", bytes.NewReader(bodyBytes))
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "test-task-id"})
			req.Header.Set(evergreen.HostHeader, "test-host-id")

			// Create and test the handler
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			handler := makeStartTask(env).(*startTaskHandler)
			require.NoError(t, handler.Parse(ctx, req))

			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			// Verify the task was updated with the correct details
			updatedTask, err := task.FindOneId(ctx, "test-task-id")
			require.NoError(t, err)
			require.NotNil(t, updatedTask)

			// Check that the task details were stored correctly
			assert.Equal(t, tc.expectedTrace, updatedTask.Details.TraceID)
			assert.Equal(t, tc.expectedDisk, updatedTask.Details.DiskDevices)
		})
	}
}
