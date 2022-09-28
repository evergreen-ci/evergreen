package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	taskSecret = "tasksecret"
)

func TestAgentFetchExpansionsForTask(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *fetchExpansionsForTaskHandler){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *fetchExpansionsForTaskHandler) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*fetchExpansionsForTaskHandler)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *fetchExpansionsForTaskHandler) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/task/t1/fetch_vars", nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "t1"})
			assert.NoError(t, rh.Parse(ctx, req))
			assert.Equal(t, rh.taskID, "t1")
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *fetchExpansionsForTaskHandler) {
			rh.taskID = "t2"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionVars)
			require.True(t, ok)
			assert.Equal(t, data.PrivateVars, map[string]bool{"b": true})
			assert.Equal(t, data.Vars, map[string]string{"a": "1", "b": "3"})
		},
		"RunSucceedsWithParamsSetOnVersion": func(ctx context.Context, t *testing.T, rh *fetchExpansionsForTaskHandler) {
			rh.taskID = "t1"
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			data, ok := resp.Data().(apimodels.ExpansionVars)
			require.True(t, ok)
			assert.Equal(t, data.PrivateVars, map[string]bool{"b": true})
			assert.Equal(t, data.Vars, map[string]string{"a": "4", "b": "3"})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, model.VersionCollection))
			t1 := task.Task{
				Id:      "t1",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334455",
			}
			t2 := task.Task{
				Id:      "t2",
				Project: "p1",
				Version: "aaaaaaaaaaff001122334456",
			}
			pRef := model.ProjectRef{
				Id: "p1",
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
				Config:     "identifier: sample",
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
				Config:     "identifier: sample",
			}
			require.NoError(t, t1.Insert())
			require.NoError(t, t2.Insert())
			require.NoError(t, pRef.Insert())
			require.NoError(t, vars.Insert())
			require.NoError(t, v1.Insert())
			require.NoError(t, v2.Insert())

			r, ok := makeFetchExpansionsForTask().(*fetchExpansionsForTaskHandler)
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

func TestAgentDataPipesConfig(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, c evergreen.DataPipesConfig){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, _ evergreen.DataPipesConfig) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentDataPipesConfig)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, _ evergreen.DataPipesConfig) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/data_pipes_config", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, c evergreen.DataPipesConfig) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.DataPipesConfig)
			require.True(t, ok)
			assert.Equal(t, data.Host, c.Host)
			assert.Equal(t, data.Region, c.Region)
			assert.Equal(t, data.AWSAccessKey, c.AWSAccessKey)
			assert.Equal(t, data.AWSSecretKey, c.AWSSecretKey)
			assert.Equal(t, data.AWSToken, c.AWSToken)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, _ evergreen.DataPipesConfig) {
			rh.config = evergreen.DataPipesConfig{}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.DataPipesConfig)
			require.True(t, ok)
			assert.Zero(t, data)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := evergreen.DataPipesConfig{
				Host:         "https://url.com",
				Region:       "us-east-1",
				AWSAccessKey: "access",
				AWSSecretKey: "secret",
				AWSToken:     "token",
			}
			r := makeAgentDataPipesConfig(c)

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
			assert.Equal(t, data.S3Bucket, s.Providers.AWS.S3.Bucket)
			assert.Equal(t, data.S3Key, s.Providers.AWS.S3.Key)
			assert.Equal(t, data.S3Secret, s.Providers.AWS.S3.Secret)
			assert.Equal(t, data.LogkeeperURL, s.LoggerConfig.LogkeeperURL)
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
						S3: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
						TaskSync: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
					},
				},
				LoggerConfig: evergreen.LoggerConfig{
					LogkeeperURL: "logkeeper_url",
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
	require.NoError(t, sampleHost.Insert())

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

	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
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
	// Set the default logger after inserting into the DB since this should
	// be set dynamically by the route handler when not set.
	projRef1.DefaultLogger = "buildlogger"

	task2 := &task.Task{
		Id:      "task2",
		Project: "project2",
	}
	projRef2 := &model.ProjectRef{
		Id:            "project2",
		DefaultLogger: "evergreen",
	}
	require.NoError(t, task2.Insert())
	require.NoError(t, projRef2.Insert())

	task3 := &task.Task{
		Id:      "task3",
		Project: "project3",
	}
	require.NoError(t, task3.Insert())

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
			taskID:         task3.Id,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "GlobalLogger",
			taskID:         task1.Id,
			expectedStatus: http.StatusOK,
			expectedData:   projRef1,
		},
		{
			name:           "ProjectLogger",
			taskID:         task2.Id,
			expectedStatus: http.StatusOK,
			expectedData:   projRef2,
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
