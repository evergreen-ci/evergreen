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
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	hostSecret = "secret"
)

func TestHostNextTask(t *testing.T) {
	distroID := "testDistro"
	buildID := "buildId"
	task1 := task.Task{
		Id:        "task1",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		BuildId:   buildID,
		Project:   "exists",
		StartTime: utility.ZeroTime,
	}
	task2 := task.Task{
		Id:        "task2",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "exists",
		BuildId:   buildID,
		StartTime: utility.ZeroTime,
	}
	task3 := task.Task{
		Id:        "task3",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "exists",
		BuildId:   buildID,
		StartTime: utility.ZeroTime,
	}
	task4 := task.Task{
		Id:        "another",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "exists",
		StartTime: utility.ZeroTime,
		BuildId:   buildID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	originalServiceFlags, err := evergreen.GetServiceFlags(ctx)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, originalServiceFlags.Set(ctx))
	}()
	newServiceFlags := *originalServiceFlags
	require.NoError(t, newServiceFlags.Set(ctx))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostAgentNextTask){
		"ShouldSucceedAndSetAgentStartTime": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			resp := rh.Run(ctx)
			assert.NotNil(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.NotNil(t, taskResp)
			assert.Equal(t, taskResp.TaskId, "task1")
			nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
			require.NoError(t, err)
			require.NotNil(t, nextTask)
			assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
			dbHost, err := host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			assert.False(t, utility.IsZeroTime(dbHost.AgentStartTime))
		},
		"ShouldExitWithOutOfDateRevisionAndTaskGroup": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			sampleHost, err := host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			require.NoError(t, sampleHost.SetAgentRevision(ctx, "out-of-date-string"))
			defer func() {
				assert.NoError(t, sampleHost.SetAgentRevision(ctx, evergreen.AgentVersion)) // reset
			}()
			rh.host = sampleHost
			rh.details = &apimodels.GetNextTaskDetails{TaskGroup: "task_group"}
			resp := rh.Run(ctx)
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.Equal(t, resp.Status(), http.StatusOK)
			assert.False(t, taskResp.ShouldExit)
		},
		"NonLegacyHostThatNeedsReprovision": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"ShouldPrepareToReprovision": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					h, err := host.FindOneId(ctx, "id")
					require.NoError(t, err)

					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(ctx, h.Id)
					require.NoError(t, err)
					assert.Equal(t, dbHost.NeedsReprovision, host.ReprovisionToNew)
					assert.Equal(t, dbHost.Status, evergreen.HostProvisioning)
					assert.False(t, dbHost.Provisioned)
					assert.False(t, dbHost.NeedsNewAgent)
					assert.True(t, dbHost.NeedsNewAgentMonitor)
					assert.True(t, utility.IsZeroTime(dbHost.AgentStartTime))
				},
				"DoesntReprovisionIfNotNeeded": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					h, err := host.FindOneId(ctx, "id")
					require.NoError(t, err)
					require.NoError(t, host.UpdateOne(ctx, bson.M{host.IdKey: h.Id}, bson.M{"$unset": bson.M{host.NeedsReprovisionKey: host.ReprovisionNone}}))
					h.NeedsReprovision = ""
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(ctx, h.Id)
					require.NoError(t, err)
					assert.Empty(t, dbHost.NeedsReprovision)
					assert.Equal(t, dbHost.Status, evergreen.HostRunning)
					assert.True(t, dbHost.Provisioned)
					assert.False(t, dbHost.NeedsNewAgent)
					assert.False(t, dbHost.NeedsNewAgentMonitor)
					assert.False(t, utility.IsZeroTime(dbHost.AgentStartTime))
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.Clear(host.Collection))
					h := host.Host{
						Id: "id",
						Distro: distro.Distro{
							Id: distroID,
							BootstrapSettings: distro.BootstrapSettings{

								Method:        distro.BootstrapMethodSSH,
								Communication: distro.CommunicationMethodRPC,
							},
						},
						Secret:           hostSecret,
						Provisioned:      true,
						Status:           evergreen.HostRunning,
						NeedsReprovision: host.ReprovisionToNew,
					}
					require.NoError(t, h.Insert(ctx))
					handler := hostAgentNextTask{
						env: env,
					}
					testCase(ctx, t, handler)
				})
			}
		},
		"IntentHost": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"ConvertsBuildingIntentHostToStartingRealHost": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					intentHost, err := host.FindOneId(ctx, "intentHost")
					require.NoError(t, err)
					instanceID := generateFakeEC2InstanceID()

					rh.host = intentHost
					rh.details = &apimodels.GetNextTaskDetails{
						AgentRevision: evergreen.AgentVersion,
						EC2InstanceID: instanceID,
					}
					resp := rh.Run(ctx)

					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
					assert.Empty(t, taskResp.TaskId)

					dbIntentHost, err := host.FindOneId(ctx, intentHost.Id)
					require.NoError(t, err)
					assert.NotNil(t, dbIntentHost)

					realHost, err := host.FindOneId(ctx, instanceID)
					require.NoError(t, err)
					assert.NotNil(t, realHost)
					assert.Equal(t, realHost.Status, evergreen.HostStarting)
				},
				"ConvertsFailedIntentHostToDecommissionedRealHost": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					intentHost, err := host.FindOneId(ctx, "intentHost")
					require.NoError(t, err)
					require.NoError(t, intentHost.SetStatus(ctx, evergreen.HostBuildingFailed, evergreen.User, ""))

					instanceID := generateFakeEC2InstanceID()
					rh.host = intentHost
					rh.details = &apimodels.GetNextTaskDetails{
						AgentRevision: evergreen.AgentVersion,
						EC2InstanceID: instanceID,
					}
					resp := rh.Run(ctx)

					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)
					assert.True(t, taskResp.ShouldExit)
					assert.Empty(t, taskResp.TaskId)

					dbIntentHost, err := host.FindOneId(ctx, "intentHost")
					require.NoError(t, err)
					assert.Nil(t, dbIntentHost)

					realHost, err := host.FindOneId(ctx, instanceID)
					require.NoError(t, err)
					assert.NotNil(t, realHost)
					assert.Equal(t, realHost.Status, evergreen.HostDecommissioned)
				},
				"ConvertsTerminatedHostIntoDecommissionedRealHost": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					intentHost, err := host.FindOneId(ctx, "intentHost")
					require.NoError(t, err)
					require.NoError(t, intentHost.SetStatus(ctx, evergreen.HostTerminated, evergreen.User, ""))

					instanceID := generateFakeEC2InstanceID()
					rh.host = intentHost
					rh.details = &apimodels.GetNextTaskDetails{
						AgentRevision: evergreen.AgentVersion,
						EC2InstanceID: instanceID,
					}
					resp := rh.Run(ctx)

					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)
					assert.Empty(t, taskResp.TaskId)

					dbIntentHost, err := host.FindOneId(ctx, "intentHost")
					require.NoError(t, err)
					assert.Nil(t, dbIntentHost)

					realHost, err := host.FindOneId(ctx, instanceID)
					require.NoError(t, err)
					assert.NotNil(t, realHost)
					assert.Equal(t, realHost.Status, evergreen.HostDecommissioned)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.Clear(host.Collection))
					intentHost := host.Host{
						Id: "intentHost",
						Distro: distro.Distro{
							Id:       distroID,
							Provider: evergreen.ProviderNameEc2Fleet,
						},
						Secret:        hostSecret,
						Provisioned:   true,
						Status:        evergreen.HostBuilding,
						AgentRevision: evergreen.AgentVersion,
						Provider:      evergreen.ProviderNameEc2Fleet,
					}
					require.NoError(t, intentHost.Insert(ctx))
					handler := hostAgentNextTask{}
					testCase(ctx, t, handler)
				})
			}
		},
		"NonLegacyHost": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			nonLegacyHost := host.Host{
				Id: "nonLegacyHost",
				Distro: distro.Distro{
					Id: distroID,
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
					},
					Provider: evergreen.ProviderNameEc2Fleet,
				},
				Secret:        hostSecret,
				Provisioned:   true,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.AgentVersion,
			}
			require.NoError(t, nonLegacyHost.Insert(ctx))

			for _, status = range []string{evergreen.HostQuarantined, evergreen.HostDecommissioned, evergreen.HostTerminated} {
				require.NoError(t, nonLegacyHost.SetStatus(ctx, status, evergreen.User, ""))
				rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
				rh.host = &nonLegacyHost
				resp := rh.Run(ctx)
				assert.NotNil(t, resp)
				assert.Equal(t, resp.Status(), http.StatusOK)
				taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
				require.True(t, ok, resp.Data())
				assert.True(t, taskResp.ShouldExit)
				assert.Empty(t, taskResp.TaskId)
				dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
				require.NoError(t, err)
				assert.Equal(t, dbHost.Status, status)
			}
		},
		"NonLegacyHostWithOldAgentRevision": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"ShouldMarkRunningWhenProvisionedByAppServer": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					require.NoError(t, nonLegacyHost.SetProvisionedNotRunning(ctx))
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = nonLegacyHost
					resp := rh.Run(ctx)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
					require.NoError(t, err)
					assert.Equal(t, dbHost.Status, evergreen.HostRunning)
				},
				"ShouldGetNextTaskWhenProvisioning": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					// setup host
					require.NoError(t, db.Update(host.Collection, bson.M{host.IdKey: nonLegacyHost.Id}, bson.M{"$set": bson.M{host.StatusKey: evergreen.HostStarting}}))
					dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
					require.NoError(t, err)
					assert.Equal(t, dbHost.Status, evergreen.HostStarting)

					// next task action
					rh.host = dbHost
					resp := rh.Run(ctx)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.NotEmpty(t, taskResp.TaskId)
					assert.Equal(t, taskResp.Build, buildID)
				},
				"LatestAgentRevisionInNextTaskDetails": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					rh.host = nonLegacyHost
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					resp := rh.Run(ctx)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
				},
				"OutdatedAgentRevisionInNextTaskDetails": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					rh.host = nonLegacyHost
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: "out-of-date"}
					resp := rh.Run(ctx)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
					handler := hostAgentNextTask{}
					nonLegacyHost := &host.Host{
						Id: "nonLegacyHost",
						Distro: distro.Distro{
							Id: distroID,
							BootstrapSettings: distro.BootstrapSettings{
								Method:        distro.BootstrapMethodUserData,
								Communication: distro.CommunicationMethodRPC,
							},
						},
						Secret:        hostSecret,
						Provisioned:   true,
						Status:        evergreen.HostRunning,
						AgentRevision: "out-of-date",
					}
					require.NoError(t, task1.Insert())
					require.NoError(t, task2.Insert())
					require.NoError(t, task3.Insert())
					require.NoError(t, task4.Insert())
					require.NoError(t, nonLegacyHost.Insert(ctx))
					handler.host = nonLegacyHost
					testCase(ctx, t, handler)
				})
			}
		},
		"WithHostThatAlreadyHasRunningTask": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"GettingNextTaskShouldReturnExistingTask": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					h2, err := host.FindOneId(ctx, "anotherHost")
					require.NoError(t, err)
					rh.host = h2
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.Equal(t, taskResp.TaskId, "existingTask")
					nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
					require.NoError(t, err)
					assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
				},
				"WithAnUndispatchedTaskButAHostThatHasThatTaskAsARunningTask": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					t1 := task.Task{
						Id:        "t1",
						Status:    evergreen.TaskUndispatched,
						Activated: true,
						BuildId:   "anotherBuild",
					}
					require.NoError(t, t1.Insert())
					anotherHost := host.Host{
						Id:            "sampleHost",
						Secret:        hostSecret,
						RunningTask:   t1.Id,
						AgentRevision: evergreen.AgentVersion,
						Provisioned:   true,
						Status:        evergreen.HostRunning,
					}
					anotherBuild := build.Build{Id: "anotherBuild"}
					require.NoError(t, anotherBuild.Insert())
					require.NoError(t, anotherHost.Insert(ctx))

					rh.host = &anotherHost
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.Equal(t, taskResp.TaskId, t1.Id)
					nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
					require.NoError(t, err)
					assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
					inactiveTask := task.Task{
						Id:        "t2",
						Status:    evergreen.TaskUndispatched,
						Activated: false,
						BuildId:   "anotherBuild",
					}
					require.NoError(t, inactiveTask.Insert())
					h3 := host.Host{
						Id:            "inactive",
						Secret:        hostSecret,
						RunningTask:   inactiveTask.Id,
						Provisioned:   true,
						Status:        evergreen.HostRunning,
						AgentRevision: evergreen.AgentVersion,
					}
					require.NoError(t, h3.Insert(ctx))
					anotherBuild = build.Build{Id: "b"}
					require.NoError(t, anotherBuild.Insert())
					rh.host = &h3
					resp = rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp = resp.Data().(apimodels.NextTaskResponse)
					assert.Equal(t, taskResp.TaskId, "")
					h, err := host.FindOne(ctx, host.ById(h3.Id))
					require.NoError(t, err)
					assert.Equal(t, h.RunningTask, "")
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
					h2 := host.Host{
						Id:            "anotherHost",
						Secret:        hostSecret,
						RunningTask:   "existingTask",
						AgentRevision: evergreen.AgentVersion,
						Provisioned:   true,
						Status:        evergreen.HostRunning,
					}
					require.NoError(t, h2.Insert(ctx))

					existingTask := task.Task{
						Id:        "existingTask",
						Status:    evergreen.TaskDispatched,
						Activated: true,
					}
					require.NoError(t, existingTask.Insert())
					handler := hostAgentNextTask{
						env: env,
					}
					testCase(ctx, t, handler)
				})
			}
		},
		"WithDegradedModeSet": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			originalServiceFlags, err := evergreen.GetServiceFlags(ctx)
			require.NoError(t, err)
			defer func() {
				// Reset to original service flags.
				assert.NoError(t, originalServiceFlags.Set(ctx))
			}()
			newServiceFlags := *originalServiceFlags
			newServiceFlags.TaskDispatchDisabled = true
			require.NoError(t, newServiceFlags.Set(ctx))
			resp := rh.Run(ctx)
			assert.NotNil(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			taskResp := resp.Data().(apimodels.NextTaskResponse)
			assert.NotNil(t, taskResp)
			assert.Equal(t, taskResp.TaskId, "")
			assert.False(t, taskResp.ShouldExit)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			colls := []string{model.ProjectRefCollection, host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection, evergreen.ConfigCollection}
			require.NoError(t, db.ClearCollections(colls...))
			defer func() {
				assert.NoError(t, db.ClearCollections(colls...))
			}()
			require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))

			tq := &model.TaskQueue{
				Distro: distroID,
				Queue: []model.TaskQueueItem{
					{Id: "task1"},
					{Id: "task2"},
					{Id: "task3"},
				},
			}

			sampleHost := host.Host{
				Id: "h1",
				Distro: distro.Distro{
					Id: distroID,
				},
				Secret:        hostSecret,
				Provisioned:   true,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.AgentVersion,
			}

			testBuild := build.Build{Id: buildID}

			pref := &model.ProjectRef{
				Id:      "exists",
				Enabled: true,
			}

			require.NoError(t, task1.Insert())
			require.NoError(t, task2.Insert())
			require.NoError(t, task3.Insert())
			require.NoError(t, task4.Insert())
			require.NoError(t, testBuild.Insert())
			require.NoError(t, pref.Insert())
			require.NoError(t, sampleHost.Insert(ctx))
			require.NoError(t, tq.Save())

			r, ok := makeHostAgentNextTask(env, nil, nil).(*hostAgentNextTask)
			require.True(t, ok)

			r.host = &sampleHost
			r.details = &apimodels.GetNextTaskDetails{}
			tCase(ctx, t, r)
		})
	}
}

func generateFakeEC2InstanceID() string {
	return "i-" + utility.RandomString()
}

func TestTaskLifecycleEndpoints(t *testing.T) {
	hostId := "h1"
	projectId := "proj"
	buildID := "b1"
	versionId := "v1"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostAgentEndTask, env *mock.Environment){
		"test task should start a background job": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			startTaskHandler := makeStartTask(env).(*startTaskHandler)
			startTaskHandler.hostID = "h1"
			startTaskHandler.taskID = "task1"
			resp := startTaskHandler.Run(ctx)
			require.Equal(t, resp.Status(), http.StatusOK)
			require.NotNil(t, resp)

			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			assert.Equal(t, 1, h.TaskCount)
		},
		"with a set of task end details indicating that task has succeeded": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
			}
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
			require.False(t, taskResp.ShouldExit)
			h, err := host.FindOne(ctx, host.ById(hostId))
			require.NoError(t, err)
			require.Equal(t, h.RunningTask, "")

			foundTask, err := task.FindOne(db.Query(task.ById("task1")))
			require.NoError(t, err)
			require.Equal(t, foundTask.Status, evergreen.TaskSucceeded)
			require.Equal(t, foundTask.Details.Status, evergreen.TaskSucceeded)

		},
		"with a set of task end details indicating that task has failed": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			testTask, err := task.FindOne(db.Query(task.ById("task1")))
			require.NoError(t, err)
			require.Equal(t, testTask.Status, evergreen.TaskStarted)
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
			require.False(t, taskResp.ShouldExit)

			h, err := host.FindOne(ctx, host.ById(hostId))
			require.NoError(t, err)
			require.Equal(t, h.RunningTask, "")

			foundTask, err := task.FindOne(db.Query(task.ById("task1")))
			require.NoError(t, err)
			require.Equal(t, foundTask.Status, evergreen.TaskFailed)
			require.Equal(t, foundTask.Details.Status, evergreen.TaskFailed)

		},
		"with a set of task end details but a task that is inactive": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			task2 := task.Task{
				Id:        "task2",
				Status:    evergreen.TaskUndispatched,
				Activated: false,
				HostId:    "h2",
				Secret:    taskSecret,
				Project:   projectId,
				BuildId:   buildID,
				Version:   versionId,
			}
			require.NoError(t, task2.Insert())

			sampleHost := host.Host{
				Id:            "h2",
				Secret:        hostSecret,
				RunningTask:   task2.Id,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.AgentVersion,
			}
			require.NoError(t, sampleHost.Insert(ctx))

			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskUndispatched,
			}
			testTask, err := task.FindOne(db.Query(task.ById("task1")))
			require.NoError(t, err)
			require.Equal(t, testTask.Status, evergreen.TaskStarted)

			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
			require.False(t, taskResp.ShouldExit)

		},
		"with tasks, a host, a build, and a task queue": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			execTask := task.Task{
				Id:           "et",
				DisplayName:  "execTask",
				Status:       evergreen.TaskStarted,
				Activated:    true,
				HostId:       "h2",
				Secret:       taskSecret,
				Project:      projectId,
				BuildId:      buildID,
				BuildVariant: "bv",
				Version:      versionId,
			}
			require.NoError(t, execTask.Insert())
			displayTask := task.Task{
				Id:             "dt",
				DisplayName:    "displayTask",
				Status:         evergreen.TaskStarted,
				Activated:      true,
				Secret:         taskSecret,
				Project:        projectId,
				BuildId:        buildID,
				Version:        versionId,
				DisplayOnly:    true,
				BuildVariant:   "bv",
				ExecutionTasks: []string{execTask.Id},
			}
			require.NoError(t, displayTask.Insert())

			sampleHost := host.Host{
				Id:            "h2",
				Secret:        hostSecret,
				RunningTask:   execTask.Id,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.AgentVersion,
			}
			require.NoError(t, sampleHost.Insert(ctx))

			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			handler.taskID = execTask.Id
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
			require.False(t, taskResp.ShouldExit)

			dbTask, err := task.FindOne(db.Query(task.ById(displayTask.Id)))
			require.NoError(t, err)
			require.Equal(t, dbTask.Status, evergreen.TaskFailed)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			colls := []string{host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection, model.ParserProjectCollection, model.ProjectRefCollection, model.VersionCollection, alertrecord.Collection, event.EventCollection}
			require.NoError(t, db.ClearCollections(colls...))
			defer func() {
				assert.NoError(t, db.ClearCollections(colls...))
			}()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			q, err := queue.NewLocalLimitedSizeSerializable(1, 1)
			require.NoError(t, err)
			env.Remote = q

			require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))

			proj := model.ProjectRef{
				Id: projectId,
			}
			parserProj := model.ParserProject{
				Id: versionId,
			}
			require.NoError(t, parserProj.Insert())
			require.NoError(t, proj.Insert())

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
					Provider: evergreen.ProviderNameMock,
				},
				Secret:                hostSecret,
				RunningTask:           task1.Id,
				Provider:              evergreen.ProviderNameStatic,
				Status:                evergreen.HostRunning,
				AgentRevision:         evergreen.AgentVersion,
				LastTaskCompletedTime: time.Now().Add(-20 * time.Minute).Round(time.Second),
			}
			require.NoError(t, sampleHost.Insert(ctx))

			testBuild := build.Build{
				Id:      buildID,
				Project: projectId,
				Version: versionId,
			}
			require.NoError(t, testBuild.Insert())

			testVersion := model.Version{
				Id:     versionId,
				Branch: projectId,
			}
			require.NoError(t, testVersion.Insert())

			r, ok := makeHostAgentEndTask(evergreen.GetEnvironment()).(*hostAgentEndTask)
			r.taskID = task1.Id
			r.hostID = hostId
			r.env = env
			require.True(t, ok)

			tCase(ctx, t, r, env)
		})
	}
}

func TestAssignNextAvailableTaskWithDispatcherSettingsVersionLegacy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	Convey("with a task queue and a host", t, func() {
		settings := distro.DispatcherSettings{
			Version: evergreen.DispatcherVersionLegacy,
		}

		colls := []string{distro.Collection, host.Collection, task.Collection, model.TaskQueuesCollection, model.ProjectRefCollection}
		require.NoError(t, db.ClearCollections(colls...))
		defer func() {
			assert.NoError(t, db.ClearCollections(colls...))
		}()
		require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))
		distroID := "testDistro"
		d := distro.Distro{
			Id:                 distroID,
			DispatcherSettings: settings,
		}
		So(d.Insert(ctx), ShouldBeNil)

		taskGroupInfo := model.TaskGroupInfo{
			Name:  "",
			Count: 2,
		}
		distroQueueInfo := model.DistroQueueInfo{
			Length:         2,
			TaskGroupInfos: []model.TaskGroupInfo{taskGroupInfo},
		}
		taskQueue := &model.TaskQueue{
			Distro: distroID,
			Queue: []model.TaskQueueItem{
				{Id: "task1"},
				{Id: "task2"},
			},
			DistroQueueInfo: distroQueueInfo,
		}
		So(taskQueue.Save(), ShouldBeNil)

		theHostWhoCanBoastTheMostRoast := host.Host{
			Id: "h1",
			Distro: distro.Distro{
				Id:                 distroID,
				DispatcherSettings: settings,
			},
			Secret: hostSecret,
			Status: evergreen.HostRunning,
		}
		So(theHostWhoCanBoastTheMostRoast.Insert(ctx), ShouldBeNil)

		task1 := task.Task{
			Id:        "task1",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
		}
		task2 := task.Task{
			Id:        "task2",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
		}
		pref := &model.ProjectRef{
			Id:      "exists",
			Enabled: true,
		}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(pref.Insert(), ShouldBeNil)

		details := &apimodels.GetNextTaskDetails{}

		Convey("a host should get the task at the top of the queue", func() {
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t.Id, ShouldEqual, "task1")

			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 1)

			h, err := host.FindOne(ctx, host.ById(theHostWhoCanBoastTheMostRoast.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, "task1")

		})
		Convey("tasks with a disabled project should be removed from the queue", func() {
			pref.Enabled = false
			So(pref.Upsert(), ShouldBeNil)

			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)

			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 0)
		})
		Convey("tasks belonging to a project with dispatching disabled should be removed from the queue", func() {
			pref.DispatchingDisabled = utility.TruePtr()
			So(pref.Upsert(), ShouldBeNil)

			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)

			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 0)
		})
		Convey("a completed task group should return a nil task", func() {
			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 2)

			details.TaskGroup = "my-task-group"
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchAliasService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
			So(shouldTeardown, ShouldBeTrue)

			// task queue unmodified
			currentTq, err = model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 2)
		})
		Convey("a task that is not undispatched should not be updated in the host", func() {
			taskQueue.Queue = []model.TaskQueueItem{
				{Id: "undispatchedTask"},
				{Id: "task2"},
			}
			So(taskQueue.Save(), ShouldBeNil)
			undispatchedTask := task.Task{
				Id:     "undispatchedTask",
				Status: evergreen.TaskStarted,
			}
			So(undispatchedTask.Insert(), ShouldBeNil)
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t.Id, ShouldEqual, "task2")

			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 0)
		})
		Convey("an empty task queue should return a nil task", func() {
			taskQueue.Queue = []model.TaskQueueItem{}
			So(taskQueue.Save(), ShouldBeNil)
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t, ShouldBeNil)
		})
		Convey("a tasks queue with a task that does not exist should continue", func() {
			taskQueue.Queue = []model.TaskQueueItem{{Id: "notatask"}}
			So(taskQueue.Save(), ShouldBeNil)
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t, ShouldBeNil)
		})
		Convey("with a host with a running task", func() {
			anotherHost := host.Host{
				Id:          "ahost",
				RunningTask: "sampleTask",
				Distro: distro.Distro{
					Id: distroID,
				},
				Secret: hostSecret,
			}
			So(anotherHost.Insert(ctx), ShouldBeNil)
			h2 := host.Host{
				Id: "host2",
				Distro: distro.Distro{
					Id: distroID,
				},
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			So(h2.Insert(ctx), ShouldBeNil)

			t1 := task.Task{
				Id:        "sampleTask",
				Status:    evergreen.TaskUndispatched,
				Project:   "exists",
				Activated: true,
			}
			So(t1.Insert(), ShouldBeNil)
			t2 := task.Task{
				Id:        "another",
				Status:    evergreen.TaskUndispatched,
				Project:   "exists",
				Activated: true,
			}
			So(t2.Insert(), ShouldBeNil)

			taskQueue.Queue = []model.TaskQueueItem{
				{Id: t1.Id},
				{Id: t2.Id},
			}
			So(taskQueue.Save(), ShouldBeNil)
			Convey("the task that is in the other host should not be assigned to another host", func() {
				t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &h2, details)
				So(err, ShouldBeNil)
				So(shouldTeardown, ShouldBeFalse)
				So(t, ShouldNotBeNil)
				So(t.Id, ShouldEqual, t2.Id)
				h, err := host.FindOne(ctx, host.ById(h2.Id))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, t2.Id)
			})
			Convey("a host with a running task should return an error", func() {
				_, _, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &anotherHost, details)
				So(err, ShouldNotBeNil)
			})
		})
		Convey("with a host running a task in a task group", func() {
			host1 := host.Host{
				Id:                      "host1",
				Status:                  evergreen.HostRunning,
				RunningTask:             "task1",
				RunningTaskGroup:        "group1",
				RunningTaskBuildVariant: "variant1",
				RunningTaskVersion:      "version1",
				RunningTaskProject:      "exists",
			}
			So(host1.Insert(ctx), ShouldBeNil)
			host2 := host.Host{
				Id:                      "host2",
				Status:                  evergreen.HostRunning,
				RunningTask:             "",
				RunningTaskGroup:        "",
				RunningTaskBuildVariant: "",
				RunningTaskVersion:      "",
				RunningTaskProject:      "",
			}
			So(host2.Insert(ctx), ShouldBeNil)
			task3 := task.Task{
				Id:                "task3",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				TaskGroup:         "group1",
				BuildVariant:      "variant1",
				Version:           "version1",
				Project:           "exists",
				TaskGroupMaxHosts: 1,
			}
			So(task3.Insert(), ShouldBeNil)
			task4 := task.Task{
				Id:                "task4",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				TaskGroup:         "group2",
				BuildVariant:      "variant1",
				Version:           "version1",
				Project:           "exists",
				TaskGroupMaxHosts: 1,
			}
			So(task4.Insert(), ShouldBeNil)
			taskQueue.Queue = []model.TaskQueueItem{
				{Id: task3.Id},
				{Id: task4.Id},
			}
			So(taskQueue.Save(), ShouldBeNil)
			t, _, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &host2, details)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			// task 3 should not be dispatched, because it's already running on max
			// hosts, instead it should be task 4
			So(t.Id, ShouldEqual, task4.Id)
			h, err := host.FindOne(ctx, host.ById(host2.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, task4.Id)
		})
		Convey("with many hosts running task group tasks", func() {
			// In this scenario likely host1 and host2 are racing, since host2 has a later
			// task group order number than what's in the queue, and will clear the running
			// task when it sees that host2 is running with a smaller task group order number.
			host1 := host.Host{
				Id:                      "host1",
				Status:                  evergreen.HostRunning,
				RunningTask:             "task1",
				RunningTaskGroup:        "group1",
				RunningTaskBuildVariant: "variant1",
				RunningTaskVersion:      "version1",
				RunningTaskProject:      "exists",
				RunningTaskGroupOrder:   2,
			}
			So(host1.Insert(ctx), ShouldBeNil)
			host2 := host.Host{
				Id:                      "host2",
				Status:                  evergreen.HostRunning,
				RunningTask:             "",
				RunningTaskGroup:        "",
				RunningTaskBuildVariant: "",
				RunningTaskVersion:      "",
				RunningTaskProject:      "",
			}
			So(host2.Insert(ctx), ShouldBeNil)
			task3 := task.Task{
				Id:                "task3",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				TaskGroup:         "group1",
				BuildVariant:      "variant1",
				Version:           "version1",
				Project:           "exists",
				TaskGroupMaxHosts: 1,
				TaskGroupOrder:    3,
			}
			So(task3.Insert(), ShouldBeNil)
			task4 := task.Task{
				Id:                "task4",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				TaskGroup:         "group1",
				BuildVariant:      "variant1",
				Version:           "version1",
				Project:           "exists",
				TaskGroupMaxHosts: 1,
				TaskGroupOrder:    1,
			}
			So(task4.Insert(), ShouldBeNil)
			taskQueue.Queue = []model.TaskQueueItem{
				{Id: task3.Id},
				{Id: task4.Id},
			}
			So(taskQueue.Save(), ShouldBeNil)
			t, _, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &host2, details)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			// task 3 should not be dispatched, because it has a later task group
			// order number than what's currently assigned to host1. Instead it should be task4.
			So(t.Id, ShouldEqual, task4.Id)
			h, err := host.FindOne(ctx, host.ById(host2.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, task4.Id)
		})
	})
}

func TestAssignNextAvailableTaskWithDispatcherSettingsVersionTunable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	Convey("with a task queue and a host", t, func() {
		settings := distro.DispatcherSettings{
			Version: evergreen.DispatcherVersionRevisedWithDependencies,
		}
		colls := []string{distro.Collection, host.Collection, task.Collection, model.TaskQueuesCollection, model.ProjectRefCollection}
		require.NoError(t, db.ClearCollections(colls...))
		defer func() {
			assert.NoError(t, db.ClearCollections(colls...))
		}()
		require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))

		d := distro.Distro{
			Id:                 "testDistro",
			DispatcherSettings: settings,
		}
		So(d.Insert(ctx), ShouldBeNil)

		taskGroupInfo := model.TaskGroupInfo{
			Name:  "",
			Count: 2,
		}
		distroQueueInfo := model.DistroQueueInfo{
			Length:         2,
			TaskGroupInfos: []model.TaskGroupInfo{taskGroupInfo},
		}
		taskQueue := &model.TaskQueue{
			Distro: d.Id,
			Queue: []model.TaskQueueItem{
				{Id: "task1"},
				{Id: "task2"},
			},
			DistroQueueInfo: distroQueueInfo,
		}
		So(taskQueue.Save(), ShouldBeNil)

		theHostWhoCanBoastTheMostRoast := host.Host{
			Id: "h1",
			Distro: distro.Distro{
				Id:                 d.Id,
				DispatcherSettings: settings,
			},
			Secret: hostSecret,
			Status: evergreen.HostRunning,
		}
		So(theHostWhoCanBoastTheMostRoast.Insert(ctx), ShouldBeNil)

		task1 := task.Task{
			Id:        "task1",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
			StartTime: utility.ZeroTime,
		}
		task2 := task.Task{
			Id:        "task2",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
			StartTime: utility.ZeroTime,
		}
		pref := &model.ProjectRef{
			Id:      "exists",
			Enabled: true,
		}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(pref.Insert(), ShouldBeNil)

		details := &apimodels.GetNextTaskDetails{}
		Convey("a host should get the task at the top of the queue", func() {
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "task1")

			currentTq, err := model.LoadTaskQueue(d.Id)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 1)

			h, err := host.FindOne(ctx, host.ById(theHostWhoCanBoastTheMostRoast.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, "task1")
		})
		Convey("a completed task group should return a nil task", func() {
			currentTq, err := model.LoadTaskQueue(d.Id)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 2)

			details.TaskGroup = "my-task-group"
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
			So(shouldTeardown, ShouldBeTrue)

			// task queue unmodified
			currentTq, err = model.LoadTaskQueue(d.Id)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 2)
			details.TaskGroup = ""
		})
		Convey("a task that is not undispatched should not be updated in the host", func() {
			taskQueue.Queue = []model.TaskQueueItem{
				{Id: "undispatchedTask"},
				{Id: "task2"},
			}
			So(taskQueue.Save(), ShouldBeNil)
			// STU: this task should never get into the queue in the first place?
			undispatchedTask := task.Task{
				Id:        "undispatchedTask",
				Status:    evergreen.TaskStarted,
				StartTime: utility.ZeroTime,
			}
			So(undispatchedTask.Insert(), ShouldBeNil)
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t.Id, ShouldEqual, "task2")

			currentTq, err := model.LoadTaskQueue(d.Id)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 0)
		})
		Convey("an empty task queue should return a nil task", func() {
			taskQueue.Queue = []model.TaskQueueItem{}
			So(taskQueue.Save(), ShouldBeNil)
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t, ShouldBeNil)
		})
		Convey("a tasks queue with a task that does not exist should error", func() {
			item := model.TaskQueueItem{
				Id:           "notatask",
				Dependencies: []string{},
			}
			taskQueue.Queue = []model.TaskQueueItem{item}
			So(taskQueue.Save(), ShouldBeNil)
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(shouldTeardown, ShouldBeFalse)
			So(t, ShouldBeNil)
		})
		Convey("with a host with a running task", func() {
			anotherHost := host.Host{
				Id:          "ahost",
				RunningTask: "sampleTask",
				Distro: distro.Distro{
					Id: d.Id,
				},
				Secret: hostSecret,
			}
			So(anotherHost.Insert(ctx), ShouldBeNil)
			h2 := host.Host{
				Id: "host2",
				Distro: distro.Distro{
					Id: d.Id,
				},
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			So(h2.Insert(ctx), ShouldBeNil)

			t1 := task.Task{
				Id:        "sampleTask",
				Status:    evergreen.TaskUndispatched,
				Project:   "exists",
				Activated: true,
				StartTime: utility.ZeroTime,
			}
			So(t1.Insert(), ShouldBeNil)
			t2 := task.Task{
				Id:        "another",
				Status:    evergreen.TaskUndispatched,
				Project:   "exists",
				Activated: true,
				StartTime: utility.ZeroTime,
			}
			So(t2.Insert(), ShouldBeNil)

			taskQueue.Queue = []model.TaskQueueItem{
				{Id: t1.Id},
				{Id: t2.Id},
			}
			So(taskQueue.Save(), ShouldBeNil)
			Convey("the task that is in the other host should not be assigned to another host", func() {
				t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &h2, details)
				So(err, ShouldBeNil)
				So(shouldTeardown, ShouldBeFalse)
				So(t, ShouldNotBeNil)
				So(t.Id, ShouldEqual, t2.Id)
				h, err := host.FindOne(ctx, host.ById(h2.Id))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, t2.Id)
			})
			Convey("a host with a running task should return an error", func() {
				_, _, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &anotherHost, details)
				So(err, ShouldNotBeNil)
			})
		})
		Convey("with a host running a task in a task group", func() {
			host1 := host.Host{
				Id:                      "host1",
				Status:                  evergreen.HostRunning,
				RunningTask:             "task1",
				RunningTaskGroup:        "group1",
				RunningTaskBuildVariant: "variant1",
				RunningTaskVersion:      "version1",
				RunningTaskProject:      "exists",
			}
			So(host1.Insert(ctx), ShouldBeNil)
			host2 := host.Host{
				Id:                      "host2",
				Status:                  evergreen.HostRunning,
				RunningTask:             "",
				RunningTaskGroup:        "",
				RunningTaskBuildVariant: "",
				RunningTaskVersion:      "",
				RunningTaskProject:      "",
			}
			So(host2.Insert(ctx), ShouldBeNil)
			task3 := task.Task{
				Id:                "task3",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				TaskGroup:         "group1",
				BuildVariant:      "variant1",
				Version:           "version1",
				Project:           "exists",
				TaskGroupMaxHosts: 1,
			}
			So(task3.Insert(), ShouldBeNil)
			task4 := task.Task{
				Id:                "task4",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				TaskGroup:         "group2",
				BuildVariant:      "variant1",
				Version:           "version1",
				Project:           "exists",
				TaskGroupMaxHosts: 1,
			}
			So(task4.Insert(), ShouldBeNil)
			taskQueue.Queue = []model.TaskQueueItem{
				{Id: task3.Id},
				{Id: task4.Id},
			}
			So(taskQueue.Save(), ShouldBeNil)
			t, _, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &host2, details)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			// task 3 should not be dispatched, because it's already running on max
			// hosts, instead it should be task 4
			So(t.Id, ShouldEqual, task4.Id)
			h, err := host.FindOne(ctx, host.ById(host2.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, task4.Id)
		})
	})
}

func TestCheckHostHealth(t *testing.T) {
	currentRevision := "abc"
	Convey("With a host that has different statuses", t, func() {
		h := &host.Host{
			Provisioned:   true,
			Status:        evergreen.HostRunning,
			AgentRevision: currentRevision,
		}
		shouldExit := checkHostHealth(h)
		So(shouldExit, ShouldBeFalse)
		h.Status = evergreen.HostDecommissioned
		shouldExit = checkHostHealth(h)
		So(shouldExit, ShouldBeTrue)
		h.Status = evergreen.HostQuarantined
		shouldExit = checkHostHealth(h)
		So(shouldExit, ShouldBeTrue)
		Convey("With a host that is running but has a different revision", func() {
			shouldExit := agentRevisionIsOld(h)
			So(shouldExit, ShouldBeTrue)
		})
	})
}

func TestHandleEndTaskForCommitQueueTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p1 := mgobson.NewObjectId().Hex()
	p2 := mgobson.NewObjectId().Hex()
	p3 := mgobson.NewObjectId().Hex()
	taskA := task.Task{
		Id:            "taskA",
		Version:       p1,
		Project:       "my_project",
		DisplayName:   "important_task",
		BuildVariant:  "best_variant",
		BuildId:       "build",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	taskB := task.Task{
		Id:            "taskB",
		Version:       p2,
		Project:       "my_project",
		DisplayName:   "important_task",
		BuildVariant:  "best_variant",
		BuildId:       "build",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	taskC := task.Task{
		Id:            "taskC",
		Version:       p3,
		Project:       "my_project",
		DisplayName:   "important_task",
		BuildVariant:  "best_variant",
		BuildId:       "build",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	for testName, testCase := range map[string]func(t *testing.T, cq commitqueue.CommitQueue){
		"next task is failed": func(t *testing.T, cq commitqueue.CommitQueue) {
			taskA.Status = evergreen.TaskSucceeded
			assert.NoError(t, taskA.Insert())

			taskB.Status = evergreen.TaskFailed
			assert.NoError(t, taskB.Insert())

			taskC.Status = evergreen.TaskFailed
			assert.NoError(t, taskC.Insert())

			// should dequeue task B and restart task C
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(ctx, &taskA, evergreen.TaskSucceeded))

			taskBFromDb, err := task.FindOneId("taskB")
			assert.NoError(t, err)
			assert.NotNil(t, taskBFromDb)
			// taskB was not restarted
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, taskBFromDb.Execution, 0)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			assert.NotNil(t, taskCFromDb)
			assert.Equal(t, evergreen.TaskUndispatched, taskCFromDb.Status)
			assert.Equal(t, taskCFromDb.Execution, 1)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			assert.NotNil(t, cqFromDb)
			assert.Equal(t, -1, cqFromDb.FindItem("taskB"))
		},
		"next task is successful": func(t *testing.T, cq commitqueue.CommitQueue) {
			taskA.Status = evergreen.TaskSucceeded
			assert.NoError(t, taskA.Insert())

			taskB.Status = evergreen.TaskSucceeded
			assert.NoError(t, taskB.Insert())

			taskC.Status = evergreen.TaskFailed
			assert.NoError(t, taskC.Insert())

			// should just restart taskC now that we know for certain taskA is the problem
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(ctx, &taskA, evergreen.TaskSucceeded))

			taskBFromDb, err := task.FindOneId("taskB")
			assert.NoError(t, err)
			assert.NotNil(t, taskBFromDb)
			// taskB was not restarted
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, taskBFromDb.Execution, 0)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			assert.NotNil(t, taskCFromDb)
			// taskC is not restarted but is dequeued
			assert.Equal(t, taskC.Status, taskCFromDb.Status)
			assert.Equal(t, taskCFromDb.Execution, 0)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			assert.NotNil(t, cqFromDb)
			assert.Equal(t, -1, cqFromDb.FindItem("taskC"))
		},
		"next task is undispatched": func(t *testing.T, cq commitqueue.CommitQueue) {
			taskA.Status = evergreen.TaskSucceeded
			assert.NoError(t, taskA.Insert())

			taskB.Status = evergreen.TaskUndispatched
			assert.NoError(t, taskB.Insert())

			// We don't know if TaskC failed because of TaskB or because of TaskA.
			taskC.Status = evergreen.TaskFailed
			assert.NoError(t, taskC.Insert())

			// shouldn't do anything since TaskB could be the problem
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(ctx, &taskA, evergreen.TaskSucceeded))

			taskBFromDb, err := task.FindOneId("taskB")
			assert.NoError(t, err)
			assert.NotNil(t, taskBFromDb)
			// taskB was not restarted
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, taskBFromDb.Execution, 0)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			assert.NotNil(t, taskCFromDb)
			// taskC was not restarted
			assert.Equal(t, taskC.Status, taskCFromDb.Status)
			assert.Equal(t, taskCFromDb.Execution, 0)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			assert.NotNil(t, cqFromDb)
			assert.Len(t, cqFromDb.Queue, 3) // no item dequeued
		},
		"next task not created yet": func(t *testing.T, cq commitqueue.CommitQueue) {
			require.Len(t, cq.Queue, 3)
			itemToChange := cq.Queue[1]
			itemToChange.Version = ""
			assert.NoError(t, cq.UpdateVersion(&itemToChange))
			assert.Empty(t, cq.Queue[1].Version)

			taskA.Status = evergreen.TaskSucceeded
			assert.NoError(t, taskA.Insert())

			// shouldn't do anything since taskB isn't scheduled
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(ctx, &taskA, evergreen.TaskSucceeded))

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			assert.NotNil(t, cqFromDb)
			assert.Len(t, cqFromDb.Queue, 3) // no item dequeued
		},
		"previous task hasn't run yet": func(t *testing.T, cq commitqueue.CommitQueue) {
			taskA.Status = evergreen.TaskDispatched
			assert.NoError(t, taskA.Insert())

			// We don't know if taskB failed because of taskA yet so we shouldn't dequeue anything.
			taskB.Status = evergreen.TaskFailed
			assert.NoError(t, taskB.Insert())

			taskC.Status = evergreen.TaskFailed
			assert.NoError(t, taskC.Insert())

			// Shouldn't do anything since TaskB could be the problem.
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(ctx, &taskB, evergreen.TaskFailed))

			// no tasks restarted
			taskBFromDb, err := task.FindOneId("taskB")
			assert.NoError(t, err)
			assert.NotNil(t, taskBFromDb)
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, taskBFromDb.Execution, 0)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			assert.NotNil(t, taskCFromDb)
			assert.Equal(t, taskC.Status, taskCFromDb.Status)
			assert.Equal(t, taskCFromDb.Execution, 0)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			assert.NotNil(t, cqFromDb)
			assert.Len(t, cqFromDb.Queue, 3) // no item dequeued
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(commitqueue.Collection, model.VersionCollection,
				task.Collection, patch.Collection, task.OldCollection, build.Collection))
			version1 := model.Version{
				Id: p1,
			}
			assert.NoError(t, version1.Insert())
			version2 := model.Version{
				Id: p2,
			}
			assert.NoError(t, version2.Insert())
			version3 := model.Version{
				Id: p3,
			}
			assert.NoError(t, version3.Insert())
			b := &build.Build{
				Id:      "build",
				Version: p1,
			}
			assert.NoError(t, b.Insert())
			patch1 := patch.Patch{
				Id: mgobson.ObjectIdHex(p1),
			}
			assert.NoError(t, patch1.Insert())
			patch2 := patch.Patch{
				Id: mgobson.ObjectIdHex(p2),
			}
			assert.NoError(t, patch2.Insert())
			patch3 := patch.Patch{
				Id: mgobson.ObjectIdHex(p3),
			}
			mergeTask1 := task.Task{
				Id:               "mergeA",
				Version:          p1,
				CommitQueueMerge: true,
				BuildId:          "build",
			}
			assert.NoError(t, mergeTask1.Insert())
			mergeTask2 := task.Task{
				Id:               "mergeB",
				Version:          p2,
				CommitQueueMerge: true,
				BuildId:          "build",
			}
			assert.NoError(t, mergeTask2.Insert())
			mergeTask3 := task.Task{
				Id:               "mergeC",
				Version:          p3,
				CommitQueueMerge: true,
				BuildId:          "build",
			}
			assert.NoError(t, mergeTask3.Insert())
			assert.NoError(t, patch3.Insert())
			cq := commitqueue.CommitQueue{
				ProjectID: "my_project",
				Queue: []commitqueue.CommitQueueItem{
					{
						Issue:   p1,
						PatchId: p1,
						Version: p1,
					},
					{
						Issue:   p2,
						PatchId: p2,
						Version: p2,
					},
					{
						Issue:   p3,
						PatchId: p3,
						Version: p3,
					},
				},
			}
			assert.NoError(t, commitqueue.InsertQueue(&cq))
			testCase(t, cq)
		})
	}

}
