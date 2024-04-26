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

const hostSecret = "secret"

func TestHostNextTask(t *testing.T) {
	distroID := "testDistro"
	buildID := "buildId"
	task1 := task.Task{
		Id:        "task1",
		Execution: 5,
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

	generateFakeEC2InstanceID := func() string {
		return "i-" + utility.RandomString()
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostAgentNextTask){
		"ShouldSucceedAndSetAgentStartTime": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			resp := rh.Run(ctx)
			assert.NotNil(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
			require.True(t, ok, resp.Data())
			assert.NotNil(t, taskResp)
			assert.Equal(t, "task1", taskResp.TaskId)
			assert.Equal(t, 5, taskResp.TaskExecution)
			nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
			require.NoError(t, err)
			require.NotNil(t, nextTask)
			assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
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
			assert.Equal(t, resp.Status(), http.StatusOK)
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
			assert.Equal(t, resp.Status(), http.StatusOK)
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
			assert.Equal(t, resp.Status(), http.StatusOK)
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
					assert.Equal(t, resp.Status(), http.StatusOK)

					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.True(t, taskResp.ShouldExit)

					dbHost, err := host.FindOneId(ctx, h.Id)
					require.NoError(t, err)
					require.NotZero(t, dbHost)
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
					require.NotZero(t, h)
					require.NoError(t, host.UpdateOne(ctx, bson.M{host.IdKey: h.Id}, bson.M{"$unset": bson.M{host.NeedsReprovisionKey: host.ReprovisionNone}}))
					h.NeedsReprovision = ""
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					rh.taskDispatcher = model.NewTaskDispatchService(time.Hour)
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(ctx, h.Id)
					require.NoError(t, err)
					require.NotZero(t, dbHost)
					assert.Empty(t, dbHost.NeedsReprovision)
					assert.Equal(t, dbHost.Status, evergreen.HostRunning)
					assert.True(t, dbHost.Provisioned)
					assert.False(t, dbHost.NeedsNewAgent)
					assert.False(t, dbHost.NeedsNewAgentMonitor)
					assert.False(t, utility.IsZeroTime(dbHost.AgentStartTime))
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, distro.Collection))
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
					require.NoError(t, h.Insert(ctx))
					require.NoError(t, h.Distro.Insert(ctx))
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
				assert.Equal(t, resp.Status(), http.StatusOK)
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
			assert.Equal(t, resp.Status(), http.StatusOK)
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
					require.NoError(t, db.Update(host.Collection, bson.M{host.IdKey: nonLegacyHost.Id}, bson.M{"$set": bson.M{host.StatusKey: evergreen.HostStarting}}))
					dbHost, err := host.FindOneId(ctx, nonLegacyHost.Id)
					require.NoError(t, err)
					assert.Equal(t, evergreen.HostStarting, dbHost.Status)

					// next task action
					rh.host = dbHost
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
					require.NoError(t, task1.Insert())
					require.NoError(t, task2.Insert())
					require.NoError(t, task3.Insert())
					require.NoError(t, task4.Insert())
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
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp, ok := resp.Data().(apimodels.NextTaskResponse)
					require.True(t, ok, resp.Data())
					assert.Equal(t, "existingTask", taskResp.TaskId)
					assert.Equal(t, 8, taskResp.TaskExecution)
					nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
					require.NoError(t, err)
					require.NotZero(t, nextTask)
					assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
					assert.Equal(t, nextTask.NumNextTaskDispatches, 3)
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
					require.NoError(t, stuckTask.Insert())
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
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusInternalServerError)

					h, err := host.FindOne(ctx, host.ById(anotherHost.Id))
					require.NoError(t, err)
					assert.Equal(t, h.RunningTask, "")

					previouslyStuckTask, err := task.FindOne(db.Query(task.ById(stuckTask.Id)))
					require.NoError(t, err)
					assert.Equal(t, previouslyStuckTask.Status, evergreen.TaskFailed)

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
					require.NotZero(t, nextTask)
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
					assert.Equal(t, "", taskResp.TaskId)
					h, err := host.FindOne(ctx, host.ById(h3.Id))
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

			colls := []string{model.ProjectRefCollection, host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection,
				evergreen.ConfigCollection, distro.Collection}
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

			require.NoError(t, d.Insert(ctx))
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
			r.taskDispatcher = model.NewTaskDispatchService(time.Hour)
			tCase(ctx, t, r)
		})
	}
}

func TestTaskLifecycleEndpoints(t *testing.T) {
	hostId := "h1"
	projectId := "proj"
	buildID := "b1"
	versionId := "v1"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostAgentEndTask, env *mock.Environment){
		"TestTaskShouldShowHostRunningTask": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			startTaskHandler := makeStartTask(env).(*startTaskHandler)
			startTaskHandler.hostID = "h1"
			startTaskHandler.taskID = "task1"
			resp := startTaskHandler.Run(ctx)
			require.Equal(t, resp.Status(), http.StatusOK)
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
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
			require.False(t, taskResp.ShouldExit)
			h, err := host.FindOne(ctx, host.ById(hostId))
			require.NoError(t, err)
			require.NotZero(t, h)
			require.Equal(t, h.RunningTask, "")

			foundTask, err := task.FindOne(db.Query(task.ById("task1")))
			require.NoError(t, err)
			require.NotZero(t, foundTask)
			require.Equal(t, evergreen.TaskSucceeded, foundTask.Status)
			require.Equal(t, evergreen.TaskSucceeded, foundTask.Details.Status)
		},
		"WithTaskEndDetailsIndicatingTaskFailed": func(ctx context.Context, t *testing.T, handler *hostAgentEndTask, env *mock.Environment) {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			testTask, err := task.FindOne(db.Query(task.ById("task1")))
			require.NoError(t, err)
			require.NotZero(t, testTask)
			require.Equal(t, evergreen.TaskStarted, testTask.Status)
			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
			require.False(t, taskResp.ShouldExit)

			h, err := host.FindOne(ctx, host.ById(hostId))
			require.NoError(t, err)
			require.NotZero(t, h)
			require.Equal(t, "", h.RunningTask)

			foundTask, err := task.FindOne(db.Query(task.ById("task1")))
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
			require.NotZero(t, testTask)
			require.Equal(t, evergreen.TaskStarted, testTask.Status)

			handler.details = *details
			resp := handler.Run(ctx)
			require.NotNil(t, resp)
			require.Equal(t, resp.Status(), http.StatusOK)
			taskResp := apimodels.EndTaskResponse{}
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
			require.NotZero(t, dbTask)
			require.Equal(t, evergreen.TaskFailed, dbTask.Status)
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
		Convey("a completed task group and an empty queue should return true for teardown", func() {
			taskQueue.Queue = []model.TaskQueueItem{}
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 0)

			details.TaskGroup = "my-task-group"
			t, shouldTeardown, err := assignNextAvailableTask(ctx, env, taskQueue, model.NewTaskDispatchService(time.Minute), &theHostWhoCanBoastTheMostRoast, details)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
			So(shouldTeardown, ShouldBeTrue)
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
				Distro: distro.Distro{
					Id: "d1",
					DispatcherSettings: distro.DispatcherSettings{
						Version: evergreen.DispatcherVersionRevisedWithDependencies,
					},
				},
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
				Distro: distro.Distro{
					Id: d.Id,
					DispatcherSettings: distro.DispatcherSettings{
						Version: evergreen.DispatcherVersionRevisedWithDependencies,
					},
				},
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
		"NextTaskIsFailed": func(t *testing.T, cq commitqueue.CommitQueue) {
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
			require.NotNil(t, taskBFromDb)
			// taskB was not restarted
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, taskBFromDb.Execution, 0)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			require.NotNil(t, taskCFromDb)
			assert.Equal(t, evergreen.TaskUndispatched, taskCFromDb.Status)
			assert.Equal(t, taskCFromDb.Execution, 1)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			require.NotNil(t, cqFromDb)
			assert.Equal(t, -1, cqFromDb.FindItem("taskB"))
		},
		"NextTaskIsSuccessful": func(t *testing.T, cq commitqueue.CommitQueue) {
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
			require.NotNil(t, taskBFromDb)
			// taskB was not restarted
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, taskBFromDb.Execution, 0)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			require.NotNil(t, taskCFromDb)
			// taskC is not restarted but is dequeued
			assert.Equal(t, taskC.Status, taskCFromDb.Status)
			assert.Equal(t, 0, taskCFromDb.Execution)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			require.NotNil(t, cqFromDb)
			assert.Equal(t, -1, cqFromDb.FindItem("taskC"))
		},
		"NextTaskIsUndispatched": func(t *testing.T, cq commitqueue.CommitQueue) {
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
			require.NotNil(t, taskBFromDb)
			// taskB was not restarted
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, 0, taskBFromDb.Execution)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			assert.NotNil(t, taskCFromDb)
			// taskC was not restarted
			assert.Equal(t, taskC.Status, taskCFromDb.Status)
			require.Equal(t, 0, taskCFromDb.Execution)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			require.NotNil(t, cqFromDb)
			assert.Len(t, cqFromDb.Queue, 3) // no item dequeued
		},
		"NextTaskIsNotCreatedYet": func(t *testing.T, cq commitqueue.CommitQueue) {
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
			require.NotNil(t, cqFromDb)
			assert.Len(t, cqFromDb.Queue, 3) // no item dequeued
		},
		"PreviousTaskHasNotRunYet": func(t *testing.T, cq commitqueue.CommitQueue) {
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
			require.NotNil(t, taskBFromDb)
			assert.Equal(t, taskB.Status, taskBFromDb.Status)
			assert.Equal(t, 0, taskBFromDb.Execution)

			taskCFromDb, err := task.FindOneId("taskC")
			assert.NoError(t, err)
			require.NotNil(t, taskCFromDb)
			assert.Equal(t, taskC.Status, taskCFromDb.Status)
			assert.Equal(t, 0, taskCFromDb.Execution)

			cqFromDb, err := commitqueue.FindOneId(cq.ProjectID)
			assert.NoError(t, err)
			require.NotNil(t, cqFromDb)
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
