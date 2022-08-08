package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	hostSecret = "secret"
	distroID   = "testDistro"
)

func TestHostNextTask(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *hostAgentNextTask){
		"ShouldSucceedAndSetAgentStartTime": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			resp := rh.Run(ctx)
			assert.NotNil(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			taskResp := resp.Data().(apimodels.NextTaskResponse)
			assert.NotNil(t, taskResp)
			assert.Equal(t, taskResp.TaskId, "task1")
			nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
			require.Nil(t, err)
			assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
			dbHost, err := host.FindOneId("h1")
			require.NoError(t, err)
			assert.False(t, utility.IsZeroTime(dbHost.AgentStartTime))
		},
		"ShouldExitWithOutOfDateRevisionAndTaskGroup": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			sampleHost, err := host.FindOneId("h1")
			require.NoError(t, err)
			require.NoError(t, sampleHost.SetAgentRevision("out-of-date-string"))
			rh.host = sampleHost
			rh.details = &apimodels.GetNextTaskDetails{TaskGroup: "task_group"}
			resp := rh.Run(ctx)
			taskResp := resp.Data().(apimodels.NextTaskResponse)
			assert.Equal(t, resp.Status(), http.StatusOK)
			assert.False(t, taskResp.ShouldExit)
			require.NoError(t, sampleHost.SetAgentRevision(evergreen.AgentVersion)) // reset
		},
		"NonLegacyHostThatNeedsReprovision": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"ShouldPrepareToReprovision": func(ctx context.Context, t *testing.T) {
					h, err := host.FindOneId("id")
					require.NoError(t, err)

					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.True(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(h.Id)
					require.NoError(t, err)
					assert.Equal(t, dbHost.NeedsReprovision, host.ReprovisionToNew)
					assert.Equal(t, dbHost.Status, evergreen.HostProvisioning)
					assert.False(t, dbHost.Provisioned)
					assert.False(t, dbHost.NeedsNewAgent)
					assert.True(t, dbHost.NeedsNewAgentMonitor)
					assert.True(t, utility.IsZeroTime(dbHost.AgentStartTime))
				},
				"DoesntReprovisionIfNotNeeded": func(ctx context.Context, t *testing.T) {
					h, err := host.FindOneId("id")
					require.NoError(t, err)
					require.NoError(t, host.UpdateOne(bson.M{host.IdKey: h.Id}, bson.M{"$unset": bson.M{host.NeedsReprovisionKey: host.ReprovisionNone}}))
					h.NeedsReprovision = ""
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = h
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(h.Id)
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
					require.NoError(t, h.Insert())
					testCase(ctx, t)
				})
			}
		},
		"IntentHost": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"ConvertsBuildingIntentHostToStartingRealHost": func(ctx context.Context, t *testing.T) {
					intentHost, err := host.FindOneId("intentHost")
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
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.False(t, taskResp.ShouldExit)
					assert.Empty(t, taskResp.TaskId)

					dbIntentHost, err := host.FindOneId(intentHost.Id)
					require.NoError(t, err)
					assert.NotNil(t, dbIntentHost)

					realHost, err := host.FindOneId(instanceID)
					require.NoError(t, err)
					assert.NotNil(t, realHost)
					assert.Equal(t, realHost.Status, evergreen.HostStarting)
				},
				"ConvertsFailedIntentHostToDecommissionedRealHost": func(ctx context.Context, t *testing.T) {
					intentHost, err := host.FindOneId("intentHost")
					require.NoError(t, err)
					require.NoError(t, intentHost.SetStatus(evergreen.HostBuildingFailed, evergreen.User, ""))

					instanceID := generateFakeEC2InstanceID()
					rh.host = intentHost
					rh.details = &apimodels.GetNextTaskDetails{
						AgentRevision: evergreen.AgentVersion,
						EC2InstanceID: instanceID,
					}
					resp := rh.Run(ctx)

					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.True(t, taskResp.ShouldExit)
					assert.True(t, taskResp.ShouldExit)
					assert.Empty(t, taskResp.TaskId)

					dbIntentHost, err := host.FindOneId("intentHost")
					require.NoError(t, err)
					assert.Nil(t, dbIntentHost)

					realHost, err := host.FindOneId(instanceID)
					require.NoError(t, err)
					assert.NotNil(t, realHost)
					assert.Equal(t, realHost.Status, evergreen.HostDecommissioned)
				},
				"ConvertsTerminatedHostIntoDecommissionedRealHost": func(ctx context.Context, t *testing.T) {
					intentHost, err := host.FindOneId("intentHost")
					require.NoError(t, err)
					require.NoError(t, intentHost.SetStatus(evergreen.HostTerminated, evergreen.User, ""))

					instanceID := generateFakeEC2InstanceID()
					rh.host = intentHost
					rh.details = &apimodels.GetNextTaskDetails{
						AgentRevision: evergreen.AgentVersion,
						EC2InstanceID: instanceID,
					}
					resp := rh.Run(ctx)

					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.True(t, taskResp.ShouldExit)
					assert.Empty(t, taskResp.TaskId)

					dbIntentHost, err := host.FindOneId("intentHost")
					require.NoError(t, err)
					assert.Nil(t, dbIntentHost)

					realHost, err := host.FindOneId(instanceID)
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
					require.NoError(t, intentHost.Insert())
					testCase(ctx, t)
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
			require.NoError(t, nonLegacyHost.Insert())

			for _, status = range []string{evergreen.HostQuarantined, evergreen.HostDecommissioned, evergreen.HostTerminated} {
				require.NoError(t, nonLegacyHost.SetStatus(status, evergreen.User, ""))
				rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
				rh.host = &nonLegacyHost
				resp := rh.Run(ctx)
				assert.NotNil(t, resp)
				assert.Equal(t, resp.Status(), http.StatusOK)
				taskResp := resp.Data().(apimodels.NextTaskResponse)
				assert.True(t, taskResp.ShouldExit)
				assert.Empty(t, taskResp.TaskId)
				dbHost, err := host.FindOneId(nonLegacyHost.Id)
				require.NoError(t, err)
				assert.Equal(t, dbHost.Status, status)
			}
		},
		"NonLegacyHostWithOldAgentRevision": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"ShouldMarkRunningWhenProvisionedByAppServer": func(ctx context.Context, t *testing.T) {
					nonLegacyHost, err := host.FindOneId("nonLegacyHost")
					require.NoError(t, err)
					require.NoError(t, nonLegacyHost.SetProvisionedNotRunning())
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					rh.host = nonLegacyHost
					resp := rh.Run(ctx)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.False(t, taskResp.ShouldExit)
					dbHost, err := host.FindOneId(nonLegacyHost.Id)
					require.NoError(t, err)
					assert.Equal(t, dbHost.Status, evergreen.HostRunning)
				},
				"ShouldGetNextTaskWhenProvisioning": func(ctx context.Context, t *testing.T) {
					nonLegacyHost, err := host.FindOneId("nonLegacyHost")
					require.NoError(t, err)
					// setup host
					require.NoError(t, db.Update(host.Collection, bson.M{host.IdKey: nonLegacyHost.Id}, bson.M{"$set": bson.M{host.StatusKey: evergreen.HostStarting}}))
					dbHost, err := host.FindOneId(nonLegacyHost.Id)
					require.NoError(t, err)
					assert.Equal(t, dbHost.Status, evergreen.HostStarting)

					// next task action
					rh.host = dbHost
					resp := rh.Run(ctx)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.NotEmpty(t, taskResp.TaskId)

					assert.Equal(t, taskResp.Build, "buildId")
				},
				"LatestAgentRevisionInNextTaskDetails": func(ctx context.Context, t *testing.T) {
					nonLegacyHost, err := host.FindOneId("nonLegacyHost")
					require.NoError(t, err)
					rh.host = nonLegacyHost
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: evergreen.AgentVersion}
					resp := rh.Run(ctx)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.False(t, taskResp.ShouldExit)
				},
				"OutdatedAgentRevisionInNextTaskDetails": func(ctx context.Context, t *testing.T) {
					nonLegacyHost, err := host.FindOneId("nonLegacyHost")
					require.NoError(t, err)
					rh.host = nonLegacyHost
					rh.details = &apimodels.GetNextTaskDetails{AgentRevision: "out-of-date"}
					resp := rh.Run(ctx)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.True(t, taskResp.ShouldExit)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
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
					require.NoError(t, nonLegacyHost.Insert())
					task1 := task.Task{
						Id:        "task1",
						Status:    evergreen.TaskUndispatched,
						Activated: true,
						BuildId:   "buildId",
						Project:   "exists",
						StartTime: utility.ZeroTime,
					}
					require.NoError(t, task1.Insert())
					testCase(ctx, t)
				})
			}
		},
		"WithHostThatAlreadyHasRunningTask": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"GettingNextTaskShouldReturnExistingTask": func(ctx context.Context, t *testing.T) {
					h2, err := host.FindOneId("anotherHost")
					require.NoError(t, err)
					rh.host = h2
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
					assert.Equal(t, taskResp.TaskId, "existingTask")
					nextTask, err := task.FindOne(db.Query(task.ById(taskResp.TaskId)))
					require.NoError(t, err)
					assert.Equal(t, nextTask.Status, evergreen.TaskDispatched)
				},
				"WithAnUndispatchedTaskButAHostThatHasThatTaskAsARunningTask": func(ctx context.Context, t *testing.T) {
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
					require.NoError(t, anotherHost.Insert())

					rh.host = &anotherHost
					resp := rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp := resp.Data().(apimodels.NextTaskResponse)
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
					require.NoError(t, h3.Insert())
					anotherBuild = build.Build{Id: "b"}
					require.NoError(t, anotherBuild.Insert())
					rh.host = &h3
					resp = rh.Run(ctx)
					assert.NotNil(t, resp)
					assert.Equal(t, resp.Status(), http.StatusOK)
					taskResp = resp.Data().(apimodels.NextTaskResponse)
					assert.Equal(t, taskResp.TaskId, "")
					h, err := host.FindOne(host.ById(h3.Id))
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
					require.NoError(t, h2.Insert())

					existingTask := task.Task{
						Id:        "existingTask",
						Status:    evergreen.TaskDispatched,
						Activated: true,
					}
					require.NoError(t, existingTask.Insert())
					testCase(ctx, t)
				})
			}
		},
		"WithDegradedModeSet": func(ctx context.Context, t *testing.T, rh *hostAgentNextTask) {
			serviceFlags := evergreen.ServiceFlags{
				TaskDispatchDisabled: true,
			}
			require.NoError(t, evergreen.SetServiceFlags(serviceFlags))
			resp := rh.Run(ctx)
			assert.NotNil(t, resp)
			assert.Equal(t, resp.Status(), http.StatusOK)
			taskResp := resp.Data().(apimodels.NextTaskResponse)
			assert.NotNil(t, taskResp)
			assert.Equal(t, taskResp.TaskId, "")
			assert.False(t, taskResp.ShouldExit)
			serviceFlags.TaskDispatchDisabled = false // unset degraded mode
			require.NoError(t, evergreen.SetServiceFlags(serviceFlags))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			colls := []string{model.ProjectRefCollection, host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection, evergreen.ConfigCollection}
			require.NoError(t, db.DropCollections(colls...))
			defer func() {
				assert.NoError(t, db.DropCollections(colls...))
			}()
			require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey))
			require.NoError(t, evergreen.SetServiceFlags(evergreen.ServiceFlags{}))

			distroID := "testDistro"
			buildID := "buildId"

			tq := &model.TaskQueue{
				Distro: distroID,
				Queue: []model.TaskQueueItem{
					{Id: "task1"},
					{Id: "task2"},
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

			testBuild := build.Build{Id: buildID}

			task3 := task.Task{
				Id:        "another",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				StartTime: utility.ZeroTime,
			}

			pref := &model.ProjectRef{
				Id:      "exists",
				Enabled: utility.TruePtr(),
			}

			require.NoError(t, task1.Insert())
			require.NoError(t, task2.Insert())
			require.NoError(t, task3.Insert())
			require.NoError(t, testBuild.Insert())
			require.NoError(t, pref.Insert())
			require.NoError(t, sampleHost.Insert())
			require.NoError(t, tq.Save())

			r, ok := makeHostAgentNextTask(evergreen.GetEnvironment(), nil, nil).(*hostAgentNextTask)
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
