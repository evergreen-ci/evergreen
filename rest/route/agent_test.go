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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	taskSecret = "tasksecret"
)

func TestAgentCedarConfig(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentCedarConfig, s *evergreen.Settings){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, s *evergreen.Settings) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentCedarConfig)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, s *evergreen.Settings) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/cedar_config", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, s *evergreen.Settings) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.CedarConfig)
			require.True(t, ok)
			assert.Equal(t, data.BaseURL, s.Cedar.BaseURL)
			assert.Equal(t, data.RPCPort, s.Cedar.RPCPort)
			assert.Equal(t, data.Username, s.Cedar.User)
			assert.Equal(t, data.APIKey, s.Cedar.APIKey)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, s *evergreen.Settings) {
			*s = evergreen.Settings{}
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

			r, ok := makeAgentCedarConfig(s).(*agentCedarConfig)
			require.True(t, ok)

			tCase(ctx, t, r, s)
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
