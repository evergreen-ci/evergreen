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
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

const hostSecret = "secret"

func TestHostNextTask(t *testing.T) {
	distroID := "testDistro"
	buildID := "buildId"
	versionID := "versionId"
	task1 := task.Task{
		Id:        "task1",
		Execution: 5,
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		BuildId:   buildID,
		Project:   "exists",
		StartTime: utility.ZeroTime,
		Version:   versionID,
	}
	task2 := task.Task{
		Id:        "task2",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "exists",
		BuildId:   buildID,
		StartTime: utility.ZeroTime,
		Version:   versionID,
	}
	task3 := task.Task{
		Id:        "task3",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "exists",
		BuildId:   buildID,
		StartTime: utility.ZeroTime,
		Version:   versionID,
	}
	task4 := task.Task{
		Id:        "another",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "exists",
		StartTime: utility.ZeroTime,
		BuildId:   buildID,
		Version:   versionID,
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
			assert.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.NotNil(t, taskResp)
			assert.Equal(t, "task1", taskResp.TaskId)
			assert.Equal(t, 5, taskResp.TaskExecution)
			nextTask, err := task.FindOneId(ctx, taskResp.TaskId)
			require.NoError(t, err)
			require.NotNil(t, nextTask)
			assert.Equal(t, evergreen.TaskDispatched, nextTask.Status)
			dbHost, err := host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.False(t, utility.IsZeroTime(dbHost.AgentStartTime))
		},
		"ShouldExitWithOutOfDateRevisionAndTaskGroup": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			sampleHost, err := host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			require.NotZero(t, sampleHost)
			require.NoError(t, sampleHost.SetAgentRevision(ctx, "out-of-date-string"))
			defer func() {
				assert.NoError(t, sampleHost.SetAgentRevision(ctx, evergreen.AgentVersion)) // reset
			}()
			rh.host = sampleHost
			rh.details = &apimodels.GetNextTaskDetails{TaskGroup: "task_group"}
			resp := rh.Run(ctx)
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.Equal(t, http.StatusOK, resp.Status())
			assert.False(t, taskResp.ShouldExit)
		},
		"SetsAndUnsetsIsTearingDown": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			sampleHost, err := host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			require.NotZero(t, sampleHost)
			rh.host = sampleHost
			rh.details = &apimodels.GetNextTaskDetails{TaskGroup: "task_group"}
			resp := rh.Run(ctx)
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.Equal(t, http.StatusOK, resp.Status())
			assert.False(t, taskResp.ShouldExit)
			assert.True(t, taskResp.ShouldTeardownGroup)

			dbHost, err := host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, dbHost.IsTearingDown())

			// unsets tearing down the next time
			rh.details = &apimodels.GetNextTaskDetails{TaskGroup: ""}
			resp = rh.Run(ctx)
			taskResp, ok = resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.Equal(t, http.StatusOK, resp.Status())
			assert.False(t, taskResp.ShouldExit)
			assert.False(t, taskResp.ShouldTeardownGroup)

			dbHost, err = host.FindOneId(ctx, "h1")
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.False(t, dbHost.IsTearingDown())
		},
		"NonLegacyHostThatNeedsReprovision": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"ShouldPrepareToReprovision": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					h, err := host.FindOneId(ctx, "id")
					require.NoError(t, err)
					require.NotZero(t, h)

					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, http.StatusOK, resp.Status())

					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)

					dbHost, err := host.FindOneId(ctx, h.Id)
					require.NoError(t, err)
					require.NotZero(t, dbHost)
					assert.Equal(t, host.ReprovisionToNew, dbHost.NeedsReprovision)
					assert.Equal(t, evergreen.HostProvisioning, dbHost.Status)
					assert.False(t, dbHost.Provisioned)
					assert.False(t, dbHost.NeedsNewAgent)
					assert.True(t, dbHost.NeedsNewAgentMonitor)
					assert.True(t, utility.IsZeroTime(dbHost.AgentStartTime))
				},
				"DoesntReprovisionIfNotNeeded": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					h, err := host.FindOneId(ctx, "id")
					require.NoError(t, err)
					require.NotZero(t, h)
					require.NoError(t, host.UpdateOne(ctx, bson.M{host.IdKey: h.Id}, bson.M{"$unset": bson.M{host.NeedsReprovisionKey: host.ReprovisionNone}}))
					h.NeedsReprovision = ""
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					rh.hostID = h.Id
					rh.taskDispatcher = model.NewTaskDispatchService(time.Hour)
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, http.StatusOK, resp.Status())
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(ctx, h.Id)
					require.NoError(t, err)
					require.NotZero(t, dbHost)
					assert.Empty(t, dbHost.NeedsReprovision)
					assert.Equal(t, evergreen.HostRunning, dbHost.Status)
					assert.True(t, dbHost.Provisioned)
					assert.False(t, dbHost.NeedsNewAgent)
					assert.False(t, dbHost.NeedsNewAgentMonitor)
					assert.False(t, utility.IsZeroTime(dbHost.AgentStartTime))
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, distro.Collection, model.VersionCollection))
					h := host.Host{
						Id: "id",
						Distro: distro.Distro{
							Id: distroID,
							BootstrapSettings: distro.BootstrapSettings{

								Method:        distro.BootstrapMethodSSH,
								Communication: distro.CommunicationMethodRPC,
							},
							DispatcherSettings: distro.DispatcherSettings{
								Version: evergreen.DispatcherVersionRevisedWithDependencies,
							},
						},
						Secret:           hostSecret,
						Provisioned:      true,
						Status:           evergreen.HostRunning,
						NeedsReprovision: host.ReprovisionToNew,
					}
					v := model.Version{
						Id: versionID,
					}
					require.NoError(t, h.Insert(ctx))
					require.NoError(t, h.Distro.Insert(ctx))
					require.NoError(t, v.Insert(t.Context()))
					handler := hostAgentNextTask{
						env: env,
					}
					testCase(ctx, t, handler)
				})
			}
		},
		"NonLegacyHost": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			require.NoError(t, db.ClearCollections(host.Collection, distro.Collection))
			nonLegacyHost := host.Host{
				Id: "nonLegacyHost",
				Distro: distro.Distro{
					Id: distroID,
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
					},
					Provider: evergreen.ProviderNameEc2Fleet,
					DispatcherSettings: distro.DispatcherSettings{
						Version: evergreen.DispatcherVersionRevisedWithDependencies,
					},
				},
				Secret:        hostSecret,
				Provisioned:   true,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.AgentVersion,
			}
			require.NoError(t, nonLegacyHost.Insert(ctx))
			require.NoError(t, nonLegacyHost.Distro.Insert(ctx))

			for _, status = range []string{evergreen.HostQuarantined, evergreen.HostDecommissioned, evergreen.HostTerminated} {
				require.NoError(t, nonLegacyHost.SetStatus(ctx, status, evergreen.User, ""))
				rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
				rh.host = &nonLegacyHost
				resp := rh.Run(ctx)
				assert.NotNil(t, resp)
				assert.Equal(t, http.StatusOK, resp.Status())
				taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
				require.True(t, ok, resp.Data())
				assert.True(t, taskResp.ShouldExit)
				assert.Empty(t, taskResp.TaskId)
				dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
				require.NoError(t, err)
				require.NotZero(t, dbHost)
				assert.Equal(t, status, dbHost.Status)
			}
		},
		"ClearsSecretOfUnresponsiveQuarantinedHost": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			h := host.Host{
				Id:     "host_id",
				Status: evergreen.HostQuarantined,
			}
			h.NeedsReprovision = host.ReprovisionToLegacy
			h.NumAgentCleanupFailures = hostAgentCleanupLimit
			require.NoError(t, h.Insert(ctx))
			rh.host = &h
			rh.details = &apimodels.GetNextTaskDetails{
				AgentRevision: evergreen.AgentVersion,
			}
			resp := rh.Run(ctx)

			assert.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.True(t, taskResp.ShouldExit)
			assert.Empty(t, taskResp.TaskId)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.Equal(t, "", dbHost.Secret)
		},
		"NonLegacyHostWithOldAgentRevision": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"ShouldMarkRunningWhenProvisionedByAppServer": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					require.NoError(t, nonLegacyHost.SetProvisionedNotRunning(ctx))
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = nonLegacyHost
					rh.hostID = nonLegacyHost.Id
					rh.taskDispatcher = model.NewTaskDispatchService(time.Hour)
					resp := rh.Run(ctx)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
					require.NoError(t, err)
					require.NotZero(t, dbHost)
					assert.Equal(t, evergreen.HostRunning, dbHost.Status)
				},
				"ShouldGetNextTaskWhenProvisioning": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					// setup host
					require.NoError(t, db.UpdateContext(ctx, host.Collection, bson.M{host.IdKey: nonLegacyHost.Id}, bson.M{"$set": bson.M{host.StatusKey: evergreen.HostStarting}}))
					dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
					require.NoError(t, err)
					assert.Equal(t, evergreen.HostStarting, dbHost.Status)

					// next task action
					rh.host = dbHost
					rh.hostID = dbHost.Id
					resp := rh.Run(ctx)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.NotEmpty(t, taskResp.TaskId)
					assert.Equal(t, buildID, taskResp.Build)
				},
				"LatestAgentRevisionInNextTaskDetails": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					nonLegacyHost, err := host.FindOneId(ctx, "nonLegacyHost")
					require.NoError(t, err)
					rh.host = nonLegacyHost
					rh.hostID = nonLegacyHost.Id
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					resp := rh.Run(ctx)
					assert.Equal(t, http.StatusOK, resp.Status())
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
					assert.Equal(t, http.StatusOK, resp.Status())
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, task.Collection, distro.Collection))
					handler := hostAgentNextTask{}
					nonLegacyHost := &host.Host{
						Id: "nonLegacyHost",
						Distro: distro.Distro{
							Id: distroID,
							DispatcherSettings: distro.DispatcherSettings{
								Version: evergreen.DispatcherVersionRevisedWithDependencies,
							},
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
					require.NoError(t, task1.Insert(t.Context()))
					require.NoError(t, task2.Insert(t.Context()))
					require.NoError(t, task3.Insert(t.Context()))
					require.NoError(t, task4.Insert(t.Context()))
					require.NoError(t, nonLegacyHost.Insert(ctx))
					require.NoError(t, nonLegacyHost.Distro.Insert(ctx))
					handler.host = nonLegacyHost
					handler.taskDispatcher = model.NewTaskDispatchService(time.Hour)
					testCase(ctx, t, handler)
				})
			}
		},
		"WithHostThatAlreadyHasRunningTask": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler hostAgentNextTask){
				"GettingNextTaskShouldReturnExistingTask": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					h2, err := host.FindOneId(ctx, "anotherHost")
					require.NoError(t, err)
					require.NotZero(t, h2)
					rh.host = h2
					rh.hostID = h2.Id
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, http.StatusOK, resp.Status())
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.Equal(t, "existingTask", taskResp.TaskId)
					assert.Equal(t, 8, taskResp.TaskExecution)
					nextTask, err := task.FindOneId(ctx, taskResp.TaskId)
					require.NoError(t, err)
					require.NotZero(t, nextTask)
					assert.Equal(t, evergreen.TaskDispatched, nextTask.Status)
					assert.Equal(t, 3, nextTask.NumNextTaskDispatches)
				},
				"AStuckNextTaskShouldError": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					stuckTask := task.Task{
						Id:                    "stuckTask",
						Status:                evergreen.TaskUndispatched,
						Activated:             true,
						BuildId:               "anotherBuild",
						NumNextTaskDispatches: 5,
						Version:               "version1",
						HostId:                "sampleHost",
					}
					require.NoError(t, stuckTask.Insert(t.Context()))
					anotherHost := host.Host{
						Id:            "sampleHost",
						Secret:        hostSecret,
						RunningTask:   stuckTask.Id,
						AgentRevision: evergreen.AgentVersion,
						Provisioned:   true,
						Status:        evergreen.HostRunning,
					}

					require.NoError(t, anotherHost.Insert(ctx))

					rh.host = &anotherHost
					rh.hostID = anotherHost.Id
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, http.StatusInternalServerError, resp.Status())

					h, err := host.FindOneId(ctx, anotherHost.Id)
					require.NoError(t, err)
					assert.Equal(t, "", h.RunningTask)

					previouslyStuckTask, err := task.FindOneId(ctx, stuckTask.Id)
					require.NoError(t, err)
					require.NotZero(t, previouslyStuckTask)
					assert.Equal(t, evergreen.TaskFailed, previouslyStuckTask.Status)

				},
				"WithAnUndispatchedTaskButAHostThatHasThatTaskAsARunningTask": func(ctx context.Context, t *testing.T, handler hostAgentNextTask) {
					t1 := task.Task{
						Id:        "t1",
						Status:    evergreen.TaskUndispatched,
						Activated: true,
						BuildId:   "anotherBuild",
					}
					require.NoError(t, t1.Insert(t.Context()))
					anotherHost := host.Host{
						Id:            "sampleHost",
						Secret:        hostSecret,
						RunningTask:   t1.Id,
						AgentRevision: evergreen.AgentVersion,
						Provisioned:   true,
						Status:        evergreen.HostRunning,
					}
					anotherBuild := build.Build{Id: "anotherBuild"}
					require.NoError(t, anotherBuild.Insert(t.Context()))
					require.NoError(t, anotherHost.Insert(ctx))

					rh.host = &anotherHost
					rh.hostID = anotherHost.Id
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, http.StatusOK, resp.Status())
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.Equal(t, taskResp.TaskId, t1.Id)
					nextTask, err := task.FindOneId(ctx, taskResp.TaskId)
					require.NoError(t, err)
					require.NotZero(t, nextTask)
					assert.Equal(t, evergreen.TaskDispatched, nextTask.Status)
					inactiveTask := task.Task{
						Id:        "t2",
						Status:    evergreen.TaskUndispatched,
						Activated: false,
						BuildId:   "anotherBuild",
					}
					require.NoError(t, inactiveTask.Insert(t.Context()))
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
					require.NoError(t, anotherBuild.Insert(t.Context()))
					rh.host = &h3
					resp = rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, http.StatusOK, resp.Status())
					taskResp = resp.Data().(apimodels.NextTaskResponse)
					assert.Equal(t, "", taskResp.TaskId)
					h, err := host.FindOneId(ctx, h3.Id)
					require.NoError(t, err)
					require.NotZero(t, h)
					assert.Equal(t, "", h.RunningTask)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
					h2 := host.Host{
						Id:                   "anotherHost",
						Secret:               hostSecret,
						RunningTask:          "existingTask",
						RunningTaskExecution: 8,
						AgentRevision:        evergreen.AgentVersion,
						Provisioned:          true,
						Status:               evergreen.HostRunning,
					}
					require.NoError(t, h2.Insert(ctx))

					existingTask := task.Task{
						Id:                    "existingTask",
						Execution:             8,
						Status:                evergreen.TaskDispatched,
						Activated:             true,
						NumNextTaskDispatches: 2,
					}
					require.NoError(t, existingTask.Insert(t.Context()))
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
			assert.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok)
			assert.NotNil(t, taskResp)
			assert.Equal(t, "", taskResp.TaskId)
			assert.False(t, taskResp.ShouldExit)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			colls := []string{model.ProjectRefCollection, host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection,
				evergreen.ConfigCollection, distro.Collection, model.VersionCollection}
			require.NoError(t, db.ClearCollections(colls...))
			defer func() {
				assert.NoError(t, db.ClearCollections(colls...))
			}()
			require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))

			tq := &model.TaskQueue{
				Distro: distroID,
				Queue: []model.TaskQueueItem{
					{Id: "task1", DependenciesMet: true},
					{Id: "task2", DependenciesMet: true},
					{Id: "task3", DependenciesMet: true},
				},
			}

			d := &distro.Distro{
				Id: distroID,
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
				},
			}

			sampleHost := host.Host{
				Id: "h1",
				Distro: distro.Distro{
					Id: distroID,
					DispatcherSettings: distro.DispatcherSettings{
						Version: evergreen.DispatcherVersionRevisedWithDependencies,
					},
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
			v := model.Version{
				Id: versionID,
			}

			require.NoError(t, d.Insert(ctx))
			require.NoError(t, task1.Insert(t.Context()))
			require.NoError(t, task2.Insert(t.Context()))
			require.NoError(t, task3.Insert(t.Context()))
			require.NoError(t, task4.Insert(t.Context()))
			require.NoError(t, testBuild.Insert(t.Context()))
			require.NoError(t, pref.Insert(t.Context()))
			require.NoError(t, sampleHost.Insert(ctx))
			require.NoError(t, tq.Save(t.Context()))
			require.NoError(t, v.Insert(t.Context()))

			r, ok := makeHostAgentNextTask(env, nil, nil).(*hostAgentNextTask)
			require.True(t, ok)

			r.host = &sampleHost
			r.hostID = sampleHost.Id
			r.details = &apimodels.GetNextTaskDetails{}
			r.taskDispatcher = model.NewTaskDispatchService(time.Hour)
			tCase(ctx, t, r)
		})
	}
}

func TestSingleTaskDistroValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	colls := []string{model.ProjectRefCollection, host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection,
		evergreen.ConfigCollection, distro.Collection, model.VersionCollection}
	require.NoError(t, db.ClearCollections(colls...))
	defer func() {
		assert.NoError(t, db.ClearCollections(colls...))
	}()
	require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	env.EvergreenSettings.SingleTaskDistro = evergreen.SingleTaskDistroConfig{
		ProjectTasksPairs: []evergreen.ProjectTasksPair{
			{
				ProjectID:    "exists",
				AllowedTasks: []string{"task1", "task3"},
			},
		},
	}
	require.NoError(t, env.EvergreenSettings.SingleTaskDistro.Set(ctx))

	d := &distro.Distro{
		Id:               "singleTaskDistro",
		SingleTaskDistro: true,
		DispatcherSettings: distro.DispatcherSettings{
			Version: evergreen.DispatcherVersionRevisedWithDependencies,
		},
	}

	tq := &model.TaskQueue{
		Distro: d.Id,
		Queue: []model.TaskQueueItem{
			{Id: "task1", DependenciesMet: true},
			{Id: "task2", DependenciesMet: true},
			{Id: "task3", DependenciesMet: true},
		},
	}

	sampleHost := host.Host{
		Id: "h1",
		Distro: distro.Distro{
			Id:               d.Id,
			SingleTaskDistro: true,
			DispatcherSettings: distro.DispatcherSettings{
				Version: evergreen.DispatcherVersionRevisedWithDependencies,
			},
		},
		Secret:        hostSecret,
		Provisioned:   true,
		Status:        evergreen.HostRunning,
		AgentRevision: evergreen.AgentVersion,
	}

	b := build.Build{Id: "buildID"}

	pref := &model.ProjectRef{
		Id:         "exists",
		Identifier: "exists",
		Enabled:    true,
	}
	v := model.Version{
		Id: "versionID",
	}

	task1 := task.Task{
		Id:          "task1",
		DisplayName: "task1",
		Status:      evergreen.TaskUndispatched,
		Activated:   true,
		BuildId:     b.Id,
		Project:     "exists",
		StartTime:   utility.ZeroTime,
		Version:     v.Id,
	}
	task2 := task.Task{
		Id:          "task2",
		DisplayName: "task2",
		Status:      evergreen.TaskUndispatched,
		Activated:   true,
		Project:     "exists",
		BuildId:     b.Id,
		StartTime:   utility.ZeroTime,
		Version:     v.Id,
	}

	require.NoError(t, d.Insert(ctx))
	require.NoError(t, task1.Insert(t.Context()))
	require.NoError(t, task2.Insert(t.Context()))
	require.NoError(t, b.Insert(t.Context()))
	require.NoError(t, pref.Insert(t.Context()))
	require.NoError(t, sampleHost.Insert(ctx))
	require.NoError(t, tq.Save(t.Context()))
	require.NoError(t, v.Insert(t.Context()))

	r, ok := makeHostAgentNextTask(env, nil, nil).(*hostAgentNextTask)
	require.True(t, ok)

	r.host = &sampleHost
	r.hostID = sampleHost.Id
	r.details = &apimodels.GetNextTaskDetails{}
	r.taskDispatcher = model.NewTaskDispatchService(time.Hour)

	// task1 will correctly pass validation and be returned by next task.
	resp := r.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
	require.True(t, ok)
	assert.Equal(t, "task1", taskResp.TaskId)
	h, err := host.FindOneId(ctx, sampleHost.Id)
	require.NoError(t, err)
	require.NotZero(t, h)
	assert.Equal(t, "task1", h.RunningTask)

	tq, err = model.LoadTaskQueue(t.Context(), d.Id)
	require.NoError(t, err)
	require.NotNil(t, tq)
	assert.Equal(t, 2, tq.Length())

	require.NoError(t, sampleHost.ClearRunningTask(ctx))

	// task2 should not be dispatched because it is not an allowed task.
	resp = r.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	taskResp, ok = resp.Data().(apimodels.NextTaskResponse)
	require.True(t, ok)
	assert.Equal(t, "", taskResp.TaskId)
	h, err = host.FindOneId(ctx, sampleHost.Id)
	require.NoError(t, err)
	require.NotZero(t, h)
	assert.Equal(t, "", h.RunningTask)

	// task2 should be taken off the queue because it is not an allowed task.
	tq, err = model.LoadTaskQueue(t.Context(), d.Id)
	require.NoError(t, err)
	require.NotNil(t, tq)
	assert.Equal(t, 1, tq.Length())

	t2, err := task.FindOneId(ctx, "task2")
	require.NoError(t, err)
	require.NotNil(t, t2)
	assert.Equal(t, evergreen.TaskFailed, t2.Status)
}

func TestHostEndTask(t *testing.T) {
	const (
		hostId    = "h1"
		projectId = "proj"
		buildID   = "b1"
		versionId = "v1"
		taskId    = "task1"
	)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostAgentEndTask, env *mock.Environment){
		"TestTaskShouldShowHostRunningTask": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			startTaskHandler := makeStartTask(env).(*startTaskHandler)
			startTaskHandler.hostID = hostId
			startTaskHandler.taskID = taskId
			resp := startTaskHandler.Run(ctx)
			require.Equal(t, http.StatusOK, resp.Status())
			require.NotNil(t, resp)

			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			assert.Equal(t, 1, h.TaskCount)
		},
		"WithTaskEndDetailsIndicatingTaskSucceeded": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
			}
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			require.False(t, taskResp.ShouldExit)
			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			require.Equal(t, "", h.RunningTask)

			foundTask, err := task.FindOneId(ctx, taskId)
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskSucceeded, foundTask.Status)
			require.Equal(t, evergreen.TaskSucceeded, foundTask.Details.Status)
		},
		"WithTaskEndDetailsIndicatingTaskFailed": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			testTask, err := task.FindOneId(ctx, taskId)
			require.NoError(t, err)
			require.NotZero(t, testTask)
			require.Equal(t, evergreen.TaskStarted, testTask.Status)
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			require.False(t, taskResp.ShouldExit)

			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			require.Equal(t, "", h.RunningTask)

			foundTask, err := task.FindOneId(ctx, taskId)
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskFailed, foundTask.Status)
			require.Equal(t, evergreen.TaskFailed, foundTask.Details.Status)
		},
		"WithTaskEndDetailsButTaskIsInactive": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
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
			require.NoError(t, task2.Insert(t.Context()))

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
			testTask, err := task.FindOneId(ctx, taskId)
			require.NoError(t, err)
			require.NotZero(t, testTask)
			require.Equal(t, evergreen.TaskStarted, testTask.Status)

			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			require.False(t, taskResp.ShouldExit)
		},
		"WithTasksHostsBuildAndTaskQueue": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
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
			require.NoError(t, execTask.Insert(t.Context()))
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
			require.NoError(t, displayTask.Insert(t.Context()))

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
			require.Equal(t, http.StatusOK, resp.Status())
			taskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			require.False(t, taskResp.ShouldExit)

			dbTask, err := task.FindOneId(ctx, displayTask.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			require.Equal(t, evergreen.TaskFailed, dbTask.Status)
		},
		"DecommissionsDynamicHostWithRepeatedSystemFailedTasks": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			require.NoError(t, host.UpdateOne(ctx, host.ById(hostId), bson.M{
				"$set": bson.M{
					host.ProviderKey: evergreen.ProviderNameEc2Fleet,
				},
			}))

			for i := 0; i < 10; i++ {
				event.LogHostTaskFinished(ctx, fmt.Sprintf("some-system-failed-task-%d", i), 0, hostId, evergreen.TaskSystemFailed)
			}

			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
				Type:   evergreen.CommandTypeSystem,
			}
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			h, err = host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			assert.Equal(t, evergreen.HostDecommissioned, h.Status, "dynamic host should be decommissioned for consecutive system failed tasks")

			foundTask, err := task.FindOneId(ctx, handler.taskID)
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskSystemFailed, foundTask.GetDisplayStatus())
		},
		"DecommissionsDynamicHostWithRepeatedSystemTimedOutTasks": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			require.NoError(t, host.UpdateOne(ctx, host.ById(hostId), bson.M{
				"$set": bson.M{
					host.ProviderKey: evergreen.ProviderNameEc2Fleet,
				},
			}))

			for i := 0; i < 10; i++ {
				event.LogHostTaskFinished(ctx, fmt.Sprintf("some-system-failed-task-%d", i), 0, hostId, evergreen.TaskSystemTimedOut)
			}

			details := &apimodels.TaskEndDetail{
				Status:   evergreen.TaskFailed,
				Type:     evergreen.CommandTypeSystem,
				TimedOut: true,
			}
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			h, err = host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			assert.Equal(t, evergreen.HostDecommissioned, h.Status, "dynamic host should be decommissioned for consecutive system failed tasks")

			foundTask, err := task.FindOneId(ctx, handler.taskID)
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskSystemTimedOut, foundTask.GetDisplayStatus())
		},
		"DecommissionsDynamicHostWithRepeatedSystemTimedUnresponsiveTasks": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			require.NoError(t, host.UpdateOne(ctx, host.ById(hostId), bson.M{
				"$set": bson.M{
					host.ProviderKey: evergreen.ProviderNameEc2Fleet,
				},
			}))

			for i := 0; i < 10; i++ {
				event.LogHostTaskFinished(ctx, fmt.Sprintf("some-system-failed-task-%d", i), 0, hostId, evergreen.TaskSystemUnresponse)
			}

			details := &apimodels.TaskEndDetail{
				Status:      evergreen.TaskFailed,
				Type:        evergreen.CommandTypeSystem,
				TimedOut:    true,
				Description: evergreen.TaskDescriptionHeartbeat,
			}
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			h, err = host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			assert.Equal(t, evergreen.HostDecommissioned, h.Status, "dynamic host should be decommissioned for consecutive system failed tasks")

			foundTask, err := task.FindOneId(ctx, handler.taskID)
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskSystemUnresponse, foundTask.GetDisplayStatus())
		},
		"SkipDecommissioningRecentlyProvisionedDynamicHostWithFailures": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			h, err := host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			for i := 0; i < 8; i++ {
				event.LogHostTaskFinished(ctx, fmt.Sprintf("some-system-failed-task-%d", i), 0, hostId, evergreen.TaskSystemUnresponse)
			}

			require.NoError(t, host.UpdateOne(ctx, host.ById(hostId), bson.M{
				"$set": bson.M{
					host.ProviderKey:      evergreen.ProviderNameEc2Fleet,
					host.ProvisionTimeKey: time.Now(), // i.e. this host was re-provisioned before two of these failures
				},
			}))

			for i := 8; i < 10; i++ {
				event.LogHostTaskFinished(ctx, fmt.Sprintf("some-system-failed-task-%d", i), 0, hostId, evergreen.TaskSystemUnresponse)
			}

			details := &apimodels.TaskEndDetail{
				Status:      evergreen.TaskFailed,
				Type:        evergreen.CommandTypeSystem,
				TimedOut:    true,
				Description: evergreen.TaskDescriptionHeartbeat,
			}
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusOK, resp.Status())
			h, err = host.FindOneId(ctx, hostId)
			require.NoError(t, err)
			require.NotZero(t, h)
			assert.NotEqual(t, evergreen.HostDecommissioned, h.Status)

			foundTask, err := task.FindOneId(ctx, handler.taskID)
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskSystemUnresponse, foundTask.GetDisplayStatus())
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
			require.NoError(t, parserProj.Insert(t.Context()))
			require.NoError(t, proj.Insert(t.Context()))

			task1 := task.Task{
				Id:        taskId,
				Status:    evergreen.TaskStarted,
				Activated: true,
				HostId:    hostId,
				Secret:    taskSecret,
				Project:   projectId,
				BuildId:   buildID,
				Version:   versionId,
			}
			require.NoError(t, task1.Insert(t.Context()))

			now := time.Now()
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
				ProvisionTime:         now.Add(-time.Hour), // provisioned before any of the events
			}
			require.NoError(t, sampleHost.Insert(ctx))

			testBuild := build.Build{
				Id:      buildID,
				Project: projectId,
				Version: versionId,
			}
			require.NoError(t, testBuild.Insert(t.Context()))

			testVersion := model.Version{
				Id:     versionId,
				Branch: projectId,
			}
			require.NoError(t, testVersion.Insert(t.Context()))

			r, ok := makeHostAgentEndTask(evergreen.GetEnvironment()).(*hostAgentEndTask)
			r.taskID = task1.Id
			r.hostID = hostId
			r.env = env
			require.True(t, ok)

			tCase(ctx, t, r, env)
		})
	}
}

func TestAssignNextAvailableTask(t *testing.T) {
	// Each section is a different unit of data (Distro -> Tq)
	type data struct {
		Project1 *model.ProjectRef
		Project2 *model.ProjectRef

		Distro1       *distro.Distro
		Host1         *host.Host
		Host2         *host.Host
		Version1      *model.Version
		BuildVariant1 *build.Build
		Tg1Task1      *task.Task
		Tg1Task2      *task.Task
		Tq1           *model.TaskQueue

		Distro2       *distro.Distro
		Host3         *host.Host
		Host4         *host.Host
		Version2      *model.Version
		BuildVariant2 *build.Build
		Task1         *task.Task
		Task2         *task.Task
		Tg2Task1      *task.Task
		Tg2Task2      *task.Task
		Tq2           *model.TaskQueue

		Distro3       *distro.Distro
		Host5         *host.Host
		Host6         *host.Host
		Version3      *model.Version
		BuildVariant3 *build.Build
		Task3         *task.Task
		Task4         *task.Task
		Tq3           *model.TaskQueue
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, d data){
		"an empty task queue should return a nil task": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			d.Tq1.Queue = []model.TaskQueueItem{}
			require.NoError(t, d.Tq1.Save(t.Context()))
			details := &apimodels.GetNextTaskDetails{}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host1, details)
			require.NoError(t, err)
			assert.Nil(t, task)
			assert.False(t, shouldTeardown)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err := host.FindOneId(ctx, d.Host1.Id)
			require.NoError(t, err)
			assert.Equal(t, "", h.RunningTask)
		},
		"an invalid task in a task queue should skip it and noop": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			d.Tq3.Queue = append([]model.TaskQueueItem{{Id: "invalid", DependenciesMet: true}}, d.Tq3.Queue...)
			require.NoError(t, d.Tq3.Save(t.Context()))
			details := &apimodels.GetNextTaskDetails{}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			// The legacy dispatcher does not automatically handle invalid tasks.
			require.NoError(t, err)
			assert.Nil(t, task)
			assert.False(t, shouldTeardown)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 3, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, "", h.RunningTask)
		},
		"a host should get the task at the top of a queue for a regular task": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{}
			nextTaskId := d.Tq3.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"a host should get the task at the top of a queue for a task group": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{}
			nextTaskId := d.Tq1.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host1, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err := host.FindOneId(ctx, d.Host1.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"tasks with a disabled project should be removed from the queue": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			// The queue has task3 then task4, task3 is under a disabled project.
			d.Project2.Enabled = false
			require.NoError(t, d.Project2.Replace(t.Context()))
			nextTaskId := "task4"
			details := &apimodels.GetNextTaskDetails{}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"tasks with a project with dispatching disabled should be removed from the queue": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			// The queue has task3 then task4, task3 is under a disabled project.
			d.Project2.DispatchingDisabled = utility.TruePtr()
			require.NoError(t, d.Project2.Replace(t.Context()))
			nextTaskId := d.Tq3.Queue[1].Id
			details := &apimodels.GetNextTaskDetails{}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"a completed task group should return a nil task if no task is available ": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{
				TaskGroup: "completed-task-group",
			}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host1, details)
			require.NoError(t, err)
			assert.Nil(t, task)
			assert.True(t, shouldTeardown)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 2, tq.Length())

			h, err := host.FindOneId(ctx, d.Host1.Id)
			require.NoError(t, err)
			assert.Equal(t, "", h.RunningTask)
		},
		"a completed task group should return the next available task when available": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{
				TaskGroup: "completed-task-group",
			}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq2, model.NewTaskDispatchService(time.Minute), d.Host3, details)
			require.NoError(t, err)
			assert.Nil(t, task)
			assert.True(t, shouldTeardown)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro2.Id)
			require.NoError(t, err)
			assert.Equal(t, 4, tq.Length())

			h, err := host.FindOneId(ctx, d.Host3.Id)
			require.NoError(t, err)
			assert.Equal(t, "", h.RunningTask)
		},
		"a dispatched task should not be updated in the host": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			require.NoError(t, task.UpdateOne(ctx, bson.M{"_id": d.Task3.Id},
				bson.M{"$set": bson.M{"status": evergreen.TaskStarted}}))
			nextTaskId := d.Tq3.Queue[1].Id
			details := &apimodels.GetNextTaskDetails{}
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"subsequentially assigning tasks to two hosts should remove them from the queue": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{}
			nextTaskId := d.Tq3.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)

			nextTaskId = d.Tq3.Queue[0].Id
			task, shouldTeardown, err = assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host6, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err = model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err = host.FindOneId(ctx, d.Host6.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"a task that is already running on another host should not be assigned again": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{}
			nextTaskId := d.Tq3.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)

			d.Tq3.Queue = append(d.Tq3.Queue, model.TaskQueueItem{Id: nextTaskId})
			nextTaskId = d.Tq3.Queue[0].Id
			task, shouldTeardown, err = assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host6, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err = model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err = host.FindOneId(ctx, d.Host6.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"a host with a running task cannot be assigned again": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{}
			nextTaskId := d.Tq3.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err := host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)

			task, shouldTeardown, err = assignNextAvailableTask(ctx, env, d.Tq3, model.NewTaskDispatchService(time.Minute), d.Host5, details)
			require.Error(t, err)
			assert.Nil(t, task)
			assert.False(t, shouldTeardown)

			tq, err = model.LoadTaskQueue(t.Context(), d.Distro3.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err = host.FindOneId(ctx, d.Host5.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)
		},
		"a host running a single host task group should be the only host assigned those tasks": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			details := &apimodels.GetNextTaskDetails{}
			nextTaskId := d.Tq1.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host1, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err := host.FindOneId(ctx, d.Host1.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)

			task, shouldTeardown, err = assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host2, details)
			fmt.Println(task, shouldTeardown, err)
			require.NoError(t, err)
			assert.Nil(t, task)
			assert.False(t, shouldTeardown)

			tq, err = model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err = host.FindOneId(ctx, d.Host2.Id)
			require.NoError(t, err)
			assert.Equal(t, "", h.RunningTask)
		},
		"multiple max hosts for task group": func(ctx context.Context, t *testing.T, env *mock.Environment, d data) {
			extraHost := &host.Host{
				Id:     "extraHost",
				Distro: *d.Distro1,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, extraHost.Insert(ctx))
			d.Tq1.Queue = append(d.Tq1.Queue, model.TaskQueueItem{Id: "tg1-task3", DependenciesMet: true})
			d.Tq1.DistroQueueInfo = model.DistroQueueInfo{
				Length:         3,
				TaskGroupInfos: []model.TaskGroupInfo{{Name: "task-group-1", Count: 3}},
			}
			require.NoError(t, d.Tq1.Save(t.Context()))
			tg1Task3 := &task.Task{
				Id:                "tg1-task3",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				Project:           d.Project1.Id,
				StartTime:         utility.ZeroTime,
				TaskGroup:         d.Tg1Task1.TaskGroup,
				Version:           d.Version1.Id,
				BuildId:           d.BuildVariant1.Id,
				BuildVariant:      d.BuildVariant1.BuildVariant,
				TaskGroupMaxHosts: 2,
			}
			require.NoError(t, tg1Task3.Insert(t.Context()))
			require.NoError(t, task.UpdateOne(ctx, bson.M{"_id": d.Tg1Task1.Id},
				bson.M{"$set": bson.M{"task_group_max_hosts": 2}}))
			require.NoError(t, task.UpdateOne(ctx, bson.M{"_id": d.Tg1Task2.Id},
				bson.M{"$set": bson.M{"task_group_max_hosts": 2}}))
			details := &apimodels.GetNextTaskDetails{}
			// The first host should get the top of the task group.
			nextTaskId := d.Tq1.Queue[0].Id
			task, shouldTeardown, err := assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host1, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err := model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 2, tq.Length())

			h, err := host.FindOneId(ctx, d.Host1.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)

			// The second host should get the next task of the task group.
			nextTaskId = d.Tq1.Queue[0].Id
			task, shouldTeardown, err = assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), d.Host2, details)
			require.NoError(t, err)
			require.NotNil(t, task)
			assert.False(t, shouldTeardown)
			assert.Equal(t, nextTaskId, task.Id)

			tq, err = model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, tq.Length())

			h, err = host.FindOneId(ctx, d.Host2.Id)
			require.NoError(t, err)
			assert.Equal(t, nextTaskId, h.RunningTask)

			// The third host should not get a task, since the limit is 2.
			task, shouldTeardown, err = assignNextAvailableTask(ctx, env, d.Tq1, model.NewTaskDispatchService(time.Minute), extraHost, details)
			fmt.Print(task, shouldTeardown, err)
			require.NoError(t, err)
			assert.Nil(t, task)
			assert.False(t, shouldTeardown)

			tq, err = model.LoadTaskQueue(t.Context(), d.Distro1.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, tq.Length())

			h, err = host.FindOneId(ctx, extraHost.Id)
			require.NoError(t, err)
			assert.Equal(t, "", h.RunningTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			colls := []string{distro.Collection, host.Collection, task.Collection, model.TaskQueuesCollection, model.ProjectRefCollection, model.VersionCollection, build.Collection}
			require.NoError(t, db.ClearCollections(colls...))
			require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))

			var data data

			data.Project1 = &model.ProjectRef{
				Id:      "exists",
				Enabled: true,
			}
			require.NoError(t, data.Project1.Insert(t.Context()))
			data.Project2 = &model.ProjectRef{
				Id:      "also-exists",
				Enabled: true,
			}
			require.NoError(t, data.Project2.Insert(t.Context()))

			data.Distro1 = &distro.Distro{
				Id: "d1",
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
				},
			}
			require.NoError(t, data.Distro1.Insert(ctx))
			data.Host1 = &host.Host{
				Id:     "h1",
				Distro: *data.Distro1,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, data.Host1.Insert(ctx))
			data.Host2 = &host.Host{
				Id:     "h2",
				Distro: *data.Distro1,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, data.Host2.Insert(ctx))
			data.Version1 = &model.Version{Id: "v1"}
			require.NoError(t, data.Version1.Insert(t.Context()))
			data.BuildVariant1 = &build.Build{
				Id:           "bv1",
				BuildVariant: "bv1",
				Version:      data.Version1.Id,
				Tasks: []build.TaskCache{
					{Id: "tg1-task1"},
					{Id: "tg1-task2"},
				},
			}
			require.NoError(t, data.BuildVariant1.Insert(t.Context()))
			data.Distro2 = &distro.Distro{
				Id: "d2",
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
				},
			}
			require.NoError(t, data.Distro2.Insert(ctx))
			data.Host3 = &host.Host{
				Id: "h3",

				Distro: *data.Distro2,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, data.Host3.Insert(ctx))
			data.Host4 = &host.Host{
				Id:     "h4",
				Distro: *data.Distro2,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, data.Host4.Insert(ctx))
			data.Version2 = &model.Version{Id: "v2"}
			require.NoError(t, data.Version2.Insert(t.Context()))
			data.BuildVariant2 = &build.Build{
				Id:           "bv2",
				BuildVariant: "bv2",
				Version:      data.Version1.Id,
				Tasks: []build.TaskCache{
					{Id: "tg2-task1"},
					{Id: "task1"},
					{Id: "task2"},
					{Id: "tg2-task2"},
				},
			}
			require.NoError(t, data.BuildVariant2.Insert(t.Context()))
			data.Distro3 = &distro.Distro{
				Id: "d3",
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
				},
			}
			require.NoError(t, data.Distro3.Insert(ctx))
			data.Host5 = &host.Host{
				Id:     "h5",
				Distro: *data.Distro3,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, data.Host5.Insert(ctx))
			data.Host6 = &host.Host{
				Id:     "h6",
				Distro: *data.Distro3,
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			require.NoError(t, data.Host6.Insert(ctx))
			data.Version3 = &model.Version{Id: "v3"}
			require.NoError(t, data.Version3.Insert(t.Context()))
			data.BuildVariant3 = &build.Build{
				Id:           "bv3",
				BuildVariant: "bv3",
				Version:      data.Version3.Id,
				Tasks: []build.TaskCache{
					{Id: "task3"},
					{Id: "task4"},
				},
			}
			require.NoError(t, data.BuildVariant3.Insert(t.Context()))
			tgInfo1 := model.TaskGroupInfo{
				Name:  "task-group-1",
				Count: 2,
			}
			data.Tg1Task1 = &task.Task{
				Id:                "tg1-task1",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				Project:           data.Project1.Id,
				StartTime:         utility.ZeroTime,
				TaskGroup:         tgInfo1.Name,
				Version:           data.Version1.Id,
				BuildId:           data.BuildVariant1.Id,
				BuildVariant:      data.BuildVariant1.BuildVariant,
				TaskGroupMaxHosts: 1,
			}
			require.NoError(t, data.Tg1Task1.Insert(t.Context()))
			data.Tg1Task2 = &task.Task{
				Id:                "tg1-task2",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				Project:           data.Project1.Id,
				StartTime:         utility.ZeroTime,
				TaskGroup:         tgInfo1.Name,
				Version:           data.Version1.Id,
				BuildId:           data.BuildVariant1.Id,
				BuildVariant:      data.BuildVariant1.BuildVariant,
				TaskGroupMaxHosts: 1,
			}
			require.NoError(t, data.Tg1Task2.Insert(t.Context()))
			data.Tq1 = &model.TaskQueue{
				Distro: data.Distro1.Id,
				Queue: []model.TaskQueueItem{
					{Id: data.Tg1Task1.Id, DependenciesMet: true},
					{Id: data.Tg1Task2.Id, DependenciesMet: true},
				},
				DistroQueueInfo: model.DistroQueueInfo{
					Length:         2,
					TaskGroupInfos: []model.TaskGroupInfo{tgInfo1},
				},
			}
			require.NoError(t, data.Tq1.Save(t.Context()))
			data.Task1 = &task.Task{
				Id:           "task1",
				Status:       evergreen.TaskUndispatched,
				Activated:    true,
				Project:      data.Project1.Id,
				StartTime:    utility.ZeroTime,
				Version:      data.Version2.Id,
				BuildId:      data.BuildVariant2.Id,
				BuildVariant: data.BuildVariant2.BuildVariant,
			}
			require.NoError(t, data.Task1.Insert(t.Context()))
			data.Task2 = &task.Task{
				Id:           "task2",
				Status:       evergreen.TaskUndispatched,
				Activated:    true,
				Project:      data.Project1.Id,
				StartTime:    utility.ZeroTime,
				Version:      data.Version2.Id,
				BuildId:      data.BuildVariant2.Id,
				BuildVariant: data.BuildVariant2.BuildVariant,
			}
			require.NoError(t, data.Task2.Insert(t.Context()))
			tgInfo2 := model.TaskGroupInfo{
				Name:  "task-group-2",
				Count: 2,
			}
			data.Tg2Task1 = &task.Task{
				Id:                "tg2-task1",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				Project:           data.Project1.Id,
				StartTime:         utility.ZeroTime,
				TaskGroup:         tgInfo2.Name,
				Version:           data.Version2.Id,
				BuildId:           data.BuildVariant2.Id,
				BuildVariant:      data.BuildVariant2.BuildVariant,
				TaskGroupMaxHosts: 1,
			}
			require.NoError(t, data.Tg2Task1.Insert(t.Context()))
			data.Tg2Task2 = &task.Task{
				Id:                "tg2-task2",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				Project:           data.Project1.Id,
				StartTime:         utility.ZeroTime,
				TaskGroup:         tgInfo2.Name,
				Version:           data.Version2.Id,
				BuildId:           data.BuildVariant2.Id,
				BuildVariant:      data.BuildVariant2.BuildVariant,
				TaskGroupMaxHosts: 1,
			}
			require.NoError(t, data.Tg2Task2.Insert(t.Context()))
			data.Tq2 = &model.TaskQueue{
				Distro: data.Distro2.Id,
				Queue: []model.TaskQueueItem{
					{Id: data.Tg2Task1.Id, DependenciesMet: true},
					{Id: data.Task1.Id, DependenciesMet: true},
					{Id: data.Task2.Id, DependenciesMet: true},
					{Id: data.Tg2Task2.Id, DependenciesMet: false},
				},
				DistroQueueInfo: model.DistroQueueInfo{
					Length:         4,
					TaskGroupInfos: []model.TaskGroupInfo{tgInfo2},
				},
			}
			require.NoError(t, data.Tq2.Save(t.Context()))
			data.Task3 = &task.Task{
				Id:           "task3",
				Status:       evergreen.TaskUndispatched,
				Activated:    true,
				Project:      data.Project2.Id,
				StartTime:    utility.ZeroTime,
				Version:      data.Version3.Id,
				BuildId:      data.BuildVariant3.Id,
				BuildVariant: data.BuildVariant3.BuildVariant,
			}
			require.NoError(t, data.Task3.Insert(t.Context()))
			data.Task4 = &task.Task{
				Id:           "task4",
				Status:       evergreen.TaskUndispatched,
				Activated:    true,
				Project:      data.Project1.Id,
				StartTime:    utility.ZeroTime,
				Version:      data.Version3.Id,
				BuildId:      data.BuildVariant3.Id,
				BuildVariant: data.BuildVariant3.BuildVariant,
			}
			require.NoError(t, data.Task4.Insert(t.Context()))
			data.Tq3 = &model.TaskQueue{
				Distro: data.Distro3.Id,
				Queue: []model.TaskQueueItem{
					{Id: data.Task3.Id, DependenciesMet: true},
					{Id: data.Task4.Id, DependenciesMet: true},
				},
				DistroQueueInfo: model.DistroQueueInfo{
					Length:         2,
					TaskGroupInfos: []model.TaskGroupInfo{},
				},
			}
			require.NoError(t, data.Tq3.Save(t.Context()))

			tCase(ctx, t, env, data)
		})
	}
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
