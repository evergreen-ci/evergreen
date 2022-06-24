package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestPodProvisioningScript(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Since the tests depend on the global service flags, reset it to its
	// initial state afterwards.
	originalFlags, err := evergreen.GetServiceFlags()
	require.NoError(t, err)
	if originalFlags.S3BinaryDownloadsDisabled {
		defer func() {
			assert.NoError(t, originalFlags.Set())
		}()
		s3ClientDownloadsEnabled := *originalFlags
		s3ClientDownloadsEnabled.S3BinaryDownloadsDisabled = false
		require.NoError(t, s3ClientDownloadsEnabled.Set())
	}

	getRoute := func(t *testing.T, settings *evergreen.Settings, podID string) *podProvisioningScript {
		rh := makePodProvisioningScript(settings)
		r, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
		require.NoError(t, err)
		r = gimlet.SetURLVars(r, map[string]string{"pod_id": podID})
		require.NoError(t, rh.Parse(ctx, r))
		pps, ok := rh.(*podProvisioningScript)
		require.True(t, ok, "route should be pod provisioning script route")
		return pps
	}

	t.Run("RunFailsWithNonexistentPod", func(t *testing.T) {
		settings := &evergreen.Settings{
			ApiUrl:            "www.test.com",
			ClientBinariesDir: "clients",
		}
		rh := getRoute(t, settings, "nonexistent")
		resp := rh.Run(ctx)
		assert.NotEqual(t, http.StatusOK, resp.Status())
	})

	t.Run("RunGeneratesScriptSuccessfully", func(t *testing.T) {
		for tName, tCase := range map[string]func(t *testing.T, settings *evergreen.Settings, p *pod.Pod){
			"EvergreenClientDownloadsWithLinuxPod": func(t *testing.T, settings *evergreen.Settings, p *pod.Pod) {
				require.NoError(t, p.Insert())
				rh := getRoute(t, settings, p.ID)
				resp := rh.Run(ctx)
				assert.Equal(t, http.StatusOK, resp.Status())

				script, ok := resp.Data().(string)
				require.True(t, ok, "route should return plaintext response")

				expected := "curl -fLO www.test.com/clients/linux_amd64/evergreen --retry 10 --retry-max-time 100 && " +
					"chmod +x evergreen && " +
					"./evergreen agent --api_server=www.test.com --mode=pod --log_prefix=/working/dir/agent --working_directory=/working/dir"
				assert.Equal(t, expected, script)
			},
			"EvergreenClientDownloadsWithWindowsPod": func(t *testing.T, settings *evergreen.Settings, p *pod.Pod) {
				p.TaskContainerCreationOpts.OS = pod.OSWindows
				require.NoError(t, p.Insert())
				rh := getRoute(t, settings, p.ID)
				resp := rh.Run(ctx)
				assert.Equal(t, http.StatusOK, resp.Status())

				script, ok := resp.Data().(string)
				require.True(t, ok, "route should return plaintext response")

				expected := "curl -fLO www.test.com/clients/windows_amd64/evergreen.exe --retry 10 --retry-max-time 100 && " +
					".\\evergreen.exe agent --api_server=www.test.com --mode=pod --log_prefix=/working/dir/agent --working_directory=/working/dir"
				assert.Equal(t, expected, script)
			},
			"S3ClientDownloadsWithLinuxPod": func(t *testing.T, settings *evergreen.Settings, p *pod.Pod) {
				require.NoError(t, p.Insert())
				settings.PodInit.S3BaseURL = "https://foo.com"

				rh := getRoute(t, settings, p.ID)
				resp := rh.Run(ctx)
				assert.Equal(t, http.StatusOK, resp.Status())

				script, ok := resp.Data().(string)
				require.True(t, ok, "route should return plaintext response")

				expected := fmt.Sprintf("(curl -fLO https://foo.com/%s/linux_amd64/evergreen --retry 10 --retry-max-time 100 || curl -fLO www.test.com/clients/linux_amd64/evergreen --retry 10 --retry-max-time 100) && "+
					"chmod +x evergreen && "+
					"./evergreen agent --api_server=www.test.com --mode=pod --log_prefix=/working/dir/agent --working_directory=/working/dir", evergreen.BuildRevision)
				assert.Equal(t, expected, script)
			},
			"S3ClientDownloadsWithWindowsPod": func(t *testing.T, settings *evergreen.Settings, p *pod.Pod) {
				p.TaskContainerCreationOpts.OS = pod.OSWindows
				require.NoError(t, p.Insert())
				settings.PodInit.S3BaseURL = "https://foo.com"

				rh := getRoute(t, settings, p.ID)
				resp := rh.Run(ctx)
				assert.Equal(t, http.StatusOK, resp.Status())

				script, ok := resp.Data().(string)
				require.True(t, ok, "route should return plaintext response")

				expected := fmt.Sprintf("(curl -fLO https://foo.com/%s/windows_amd64/evergreen.exe --retry 10 --retry-max-time 100 || curl -fLO www.test.com/clients/windows_amd64/evergreen.exe --retry 10 --retry-max-time 100) && "+
					".\\evergreen.exe agent --api_server=www.test.com --mode=pod --log_prefix=/working/dir/agent --working_directory=/working/dir", evergreen.BuildRevision)
				assert.Equal(t, expected, script)
			},
		} {
			t.Run(tName, func(t *testing.T) {
				require.NoError(t, db.ClearCollections(pod.Collection))
				p := &pod.Pod{
					ID: "id",
					TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
						OS:         pod.OSLinux,
						Arch:       pod.ArchAMD64,
						WorkingDir: "/working/dir",
					},
					Status: pod.StatusStarting,
					TimeInfo: pod.TimeInfo{
						Initializing: time.Now().Add(-2 * time.Minute),
						Starting:     time.Now().Add(-time.Minute),
					},
				}
				settings := &evergreen.Settings{
					ApiUrl:            "www.test.com",
					ClientBinariesDir: "clients",
				}
				tCase(t, settings, p)
			})
		}
	})
}

func TestPodAgentSetup(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*podAgentSetup)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/pod/agent/setup", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.AgentSetupData)
			require.True(t, ok)
			assert.Equal(t, data.SplunkServerURL, s.Splunk.ServerURL)
			assert.Equal(t, data.SplunkClientToken, s.Splunk.Token)
			assert.Equal(t, data.SplunkChannel, s.Splunk.Channel)
			assert.Equal(t, data.S3Bucket, s.Providers.AWS.S3.Bucket)
			assert.Equal(t, data.S3Key, s.Providers.AWS.S3.Key)
			assert.Equal(t, data.S3Secret, s.Providers.AWS.S3.Secret)
			assert.Equal(t, data.LogkeeperURL, s.LoggerConfig.LogkeeperURL)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
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
				Splunk: send.SplunkConnectionInfo{
					ServerURL: "server_url",
					Token:     "token",
					Channel:   "channel",
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

			r, ok := makePodAgentSetup(s).(*podAgentSetup)
			require.True(t, ok)

			tCase(ctx, t, r, s)
		})
	}
}

func TestPodAgentNextTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, event.AllLogCollection, model.ProjectRefCollection))
	}()
	getPod := func() pod.Pod {
		return pod.Pod{
			ID:     "pod1",
			Status: pod.StatusRunning,
		}
	}
	getTask := func() task.Task {
		return task.Task{
			Id:                 "t1",
			Project:            "proj",
			ContainerAllocated: true,
			ExecutionPlatform:  task.ExecutionPlatformContainer,
			Status:             evergreen.TaskUndispatched,
			Activated:          true,
		}
	}
	getProject := func() model.ProjectRef {
		return model.ProjectRef{
			Id:      "proj",
			Enabled: utility.ToBoolPtr(true),
		}
	}
	getDispatcher := func() dispatcher.PodDispatcher {
		return dispatcher.PodDispatcher{
			ID:                primitive.NewObjectID().Hex(),
			PodIDs:            []string{"pod1"},
			TaskIDs:           []string{"t1"},
			ModificationCount: 0,
		}
	}
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment){
		"ParseSetsPodID": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			r, err := http.NewRequest(http.MethodGet, "/url", nil)
			require.NoError(t, err)
			podID := "some_pod_id"
			r = gimlet.SetURLVars(r, map[string]string{"pod_id": podID})
			require.NoError(t, rh.Parse(ctx, r))
			assert.Equal(t, podID, rh.podID)
		},
		"ParseFailsWithoutPodID": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			r, err := http.NewRequest(http.MethodGet, "/url", nil)
			require.NoError(t, err)
			assert.Error(t, rh.Parse(ctx, r))
			assert.Zero(t, rh.podID)
		},
		"RunFailsWithNonexistentPod": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			d := getDispatcher()
			require.NoError(t, d.Insert())
			rh.podID = "pod1"
			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"RunFailsWithNonexistentDispatcher": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			require.NoError(t, p.Insert())
			rh.podID = p.ID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"RunShouldExitWithNonRunningPod": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			p.Status = pod.StatusStarting
			require.NoError(t, p.Insert())
			rh.podID = p.ID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			nextTaskResp := resp.Data().(*apimodels.NextTaskResponse)
			assert.Equal(t, nextTaskResp.ShouldExit, true)
		},
		"RunCorrectlyMarksContainerTaskDispatched": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			require.NoError(t, p.Insert())
			d := getDispatcher()
			require.NoError(t, d.Insert())
			proj := getProject()
			require.NoError(t, proj.Insert())
			tsk := getTask()
			require.NoError(t, tsk.Insert())
			rh.podID = p.ID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			nextTaskResp := resp.Data().(*apimodels.NextTaskResponse)
			assert.Equal(t, nextTaskResp.ShouldExit, false)
			assert.Equal(t, nextTaskResp.TaskId, "t1")
			foundTask, err := task.FindOneId("t1")
			require.NoError(t, err)
			assert.Equal(t, foundTask.Status, evergreen.TaskDispatched)
		},
		"RunEnqueuesTerminationJobWhenThereAreNoTasksToDispatch": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			require.NoError(t, p.Insert())
			rh.podID = p.ID

			d := getDispatcher()
			require.NoError(t, d.Insert())

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotZero(t, dbPod.TimeInfo.AgentStarted)

			stats := env.RemoteQueue().Stats(ctx)
			assert.Equal(t, 1, stats.Total)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			require.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, event.AllLogCollection, model.ProjectRefCollection))

			rh, ok := makePodAgentNextTask(env).(*podAgentNextTask)
			require.True(t, ok)

			tCase(ctx, t, rh, env)
		})
	}
}
