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
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
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
			assert.Equal(t, rh.taskID, "t1")
			assert.Equal(t, rh.hostID, "host_id")
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			rh.taskID = "t2"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionsAndVars)
			require.True(t, ok)
			assert.Equal(t, rh.taskID, data.Expansions.Get("task_id"))
			assert.Equal(t, data.Vars, map[string]string{"a": "1", "b": "3"})
			assert.Equal(t, data.PrivateVars, map[string]bool{"b": true})
			assert.Equal(t, data.RedactKeys, []string{"pass", "secret"})
		},
		"RunSucceedsWithParamsSetOnVersion": func(ctx context.Context, t *testing.T, rh *getExpansionsAndVarsHandler) {
			rh.taskID = "t1"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionsAndVars)
			require.True(t, ok)
			assert.Equal(t, data.Vars, map[string]string{"a": "4", "b": "3"})
			assert.Equal(t, data.PrivateVars, map[string]bool{"b": true})
			assert.Equal(t, data.RedactKeys, []string{"pass", "secret"})
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
			assert.Equal(t, "password", data.Expansions.Get(evergreen.HostServicePasswordExpansion))
			assert.Equal(t, data.Vars, map[string]string{"a": "4", "b": "3"})
			assert.Equal(t, data.PrivateVars, map[string]bool{"b": true})
			assert.Equal(t, data.RedactKeys, []string{"pass", "secret"})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.Settings().LoggerConfig.RedactKeys = []string{"pass", "secret"}

			testutil.ConfigureIntegrationTest(t, env.Settings(), t.Name())

			require.NoError(t, db.ClearCollections(host.Collection, task.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, model.VersionCollection, model.ParserProjectCollection))

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
			require.NoError(t, t1.Insert())
			require.NoError(t, t2.Insert())
			require.NoError(t, pRef.Insert())
			require.NoError(t, vars.Insert())
			require.NoError(t, v1.Insert())
			require.NoError(t, v2.Insert())
			require.NoError(t, pp1.Insert())
			require.NoError(t, pp2.Insert())
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
			assert.Equal(t, rh.taskID, "t1")
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			rh.taskID = "t2"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err := task.FindOneId("t2")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.True(t, foundTask.ResetWhenFinished)
			assert.True(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)

			// Should fail if the task resets and tries to auto restart again
			require.NoError(t, foundTask.MarkEnd(time.Now(), &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}))
			require.NoError(t, foundTask.Archive(ctx))
			require.NoError(t, foundTask.Reset(ctx, ""))
			resp = rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusBadRequest, resp.Status())

			foundTask, err = task.FindOneId("t2")
			require.NoError(t, err)
			require.NotNil(t, foundTask)
			assert.False(t, foundTask.ResetWhenFinished)
			assert.False(t, foundTask.IsAutomaticRestart)
			assert.Equal(t, foundTask.Status, evergreen.TaskUndispatched)
			assert.Equal(t, 1, foundTask.NumAutomaticRestarts)
		},
		"RunSucceedsWithDisplayTask": func(ctx context.Context, t *testing.T, rh *markTaskForRestartHandler) {
			rh.taskID = "et1"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			foundTask, err := task.FindOneId("dt")
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

			foundTask, err = task.FindOneId("dt")
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

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			require.NoError(t, db.ClearCollections(task.Collection))

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
			require.NoError(t, et1.Insert())
			require.NoError(t, et2.Insert())
			require.NoError(t, dt.Insert())
			require.NoError(t, t2.Insert())
			r, ok := makeMarkTaskForRestart().(*markTaskForRestartHandler)
			require.True(t, ok)

			tCase(ctx, t, r)
		})
	}
}

func TestAgentCedarConfig(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentCedarConfig)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/cedar_config", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.CedarConfig)
			require.True(t, ok)
			assert.Equal(t, data.BaseURL, c.BaseURL)
			assert.Equal(t, data.RPCPort, c.RPCPort)
			assert.Equal(t, data.Username, c.User)
			assert.Equal(t, data.APIKey, c.APIKey)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, _ evergreen.CedarConfig) {
			rh.config = evergreen.CedarConfig{}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.CedarConfig)
			require.True(t, ok)
			assert.Zero(t, data)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := evergreen.CedarConfig{
				BaseURL: "url.com",
				RPCPort: "9090",
				User:    "user",
				APIKey:  "key",
			}
			r := makeAgentCedarConfig(c)

			tCase(ctx, t, r, c)
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
			assert.Equal(t, data.TaskSync, s.Providers.AWS.TaskSync)
			assert.Equal(t, data.EC2Keys, s.Providers.AWS.EC2Keys)
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
						TaskSync: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
						EC2Keys: []evergreen.EC2Key{
							{
								Name:   "ec2-key",
								Region: "us-east-1",
								Key:    "key",
								Secret: "secret",
							},
						},
					},
				},
			}

			r, ok := makeAgentSetup(s).(*agentSetup)
			require.True(t, ok)

			tCase(ctx, t, r, s)
		})
	}
}

func TestAgentCheckGetPullRequestHandler(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentCheckGetPullRequestHandler, s *evergreen.Settings){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentCheckGetPullRequestHandler, s *evergreen.Settings) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentCheckGetPullRequestHandler)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentCheckGetPullRequestHandler, s *evergreen.Settings) {
			json := []byte(`{
                "owner": "evergreen",
                "repo": "sandbox"
            }`)
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/task/t1/pull_request", bytes.NewBuffer(json))
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "t1"})
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
			assert.Equal(t, "t1", rh.taskID)
			assert.Equal(t, "evergreen", rh.req.Owner)
			assert.Equal(t, "sandbox", rh.req.Repo)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(patch.Collection, task.Collection))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			assert.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Credentials = map[string]string{"github": "token globalGitHubOauthToken"}

			tsk := &task.Task{
				Id:      "t1",
				Version: "aaaaaaaaaaff001122334456",
			}
			patch := &patch.Patch{
				Id:      patch.NewId("aaaaaaaaaaff001122334456"),
				Version: "aaaaaaaaaaff001122334456",
				GithubPatchData: thirdparty.GithubPatch{
					MergeCommitSHA: "abc",
				},
			}
			require.NoError(t, tsk.Insert())
			require.NoError(t, patch.Insert())

			r, ok := makeAgentGetPullRequest(env.Settings()).(*agentCheckGetPullRequestHandler)
			require.True(t, ok)

			tCase(ctx, t, r, env.Settings())
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
	require.NoError(t, parentPatch.Insert())

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
	require.NoError(t, task1.Insert())

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
	assert.Equal(t, resp.Status(), http.StatusOK)

	p, err := patch.FindOneId(parentPatchId)
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
	require.NoError(t, task1.Insert())
	require.NoError(t, projRef1.Insert())

	task2 := &task.Task{
		Id:      "task2",
		Project: "project2",
	}
	require.NoError(t, task2.Insert())

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

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *createInstallationToken, env *mock.Environment){
		"ParseErrorsOnEmptyOwnerAndRepo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", "", "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(ctx, request))
		},
		"ParseErrorsOnEmptyOwner": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", "", validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing owner")
		},
		"ParseErrorsOnEmptyRepo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", validOwner, "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing repo")
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
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

			r, ok := makeCreateInstallationToken(env).(*createInstallationToken)
			require.True(t, ok)

			tCase(ctx, t, r, env)
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
	require.NoError(t, patch.Insert())

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
	require.NoError(t, task1.Insert())

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
	assert.Equal(t, r.taskID, "task1")

	assert.Equal(t, utility.FromStringPtr(r.checkRunOutput.Title), "This is my report")
	assert.Equal(t, len(r.checkRunOutput.Annotations), 1)
}

func TestCreateGitHubDynamicAccessToken(t *testing.T) {
	route := "/task/%s/github_dynamic_access_token/%s/%s"
	owner := "owner"
	repo := "repo"
	taskID := "taskID"
	json := []byte(`{ "checks": "read", "actions": "write" }`)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *createGitHubDynamicAccessToken, env *mock.Environment){
		"ParseErrorsOnEmptyOwner": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken, env *mock.Environment) {
			url := fmt.Sprintf(route, taskID, "", repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": "", "repo": repo, "taskID": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing owner")
		},
		"ParseErrorsOnEmptyRepo": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken, env *mock.Environment) {
			url := fmt.Sprintf(route, taskID, owner, "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": "", "taskID": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing repo")
		},
		"ParseErrorsOnEmptyTaskID": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken, env *mock.Environment) {
			url := fmt.Sprintf(route, "", owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "taskID": ""}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "missing taskID")
		},
		"ParseErrorsOnEmptyBody": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken, env *mock.Environment) {
			url := fmt.Sprintf(route, taskID, owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "taskID": taskID}
			request = gimlet.SetURLVars(request, options)

			assert.ErrorContains(t, handler.Parse(ctx, request), "reading permissions body for task")
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *createGitHubDynamicAccessToken, env *mock.Environment) {
			url := fmt.Sprintf(route, taskID, owner, repo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(json))
			require.NoError(t, err)

			options := map[string]string{"owner": owner, "repo": repo, "taskID": taskID}
			request = gimlet.SetURLVars(request, options)

			require.NoError(t, handler.Parse(ctx, request))
			require.NotNil(t, handler.permissions)
			assert.Equal(t, utility.FromStringPtr(handler.permissions.Checks), "read")
			assert.Equal(t, utility.FromStringPtr(handler.permissions.Actions), "write")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			r, ok := makeCreateGitHubDynamicAccessToken(env).(*createGitHubDynamicAccessToken)
			require.True(t, ok)

			tCase(ctx, t, r, env)
		})
	}
}
