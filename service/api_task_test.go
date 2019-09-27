package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

var (
	hostSecret = "secret"
	taskSecret = "tasksecret"
)

func getNextTaskEndpoint(t *testing.T, as *APIServer, hostId string, details *apimodels.GetNextTaskDetails) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.GetServiceApp().Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := "/api/2/agent/next_task"

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.Header.Add(evergreen.HostHeader, hostId)
	request.Header.Add(evergreen.HostSecretHeader, hostSecret)
	jsonBytes, err := json.Marshal(*details)
	require.NoError(t, err, "error marshalling json")
	request.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
}

func getEndTaskEndpoint(t *testing.T, as *APIServer, hostId, taskId string, details *apimodels.TaskEndDetail) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.GetServiceApp().Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := fmt.Sprintf("/api/2/task/%s/end", taskId)

	request, err := http.NewRequest("POST", url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.Header.Add(evergreen.HostHeader, hostId)
	request.Header.Add(evergreen.HostSecretHeader, hostSecret)
	request.Header.Add(evergreen.TaskSecretHeader, taskSecret)

	jsonBytes, err := json.Marshal(*details)
	require.NoError(t, err, "error marshalling json")
	request.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
}

func getStartTaskEndpoint(t *testing.T, as *APIServer, hostId, taskId string) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.GetServiceApp().Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := fmt.Sprintf("/api/2/task/%v/start", taskId)

	request, err := http.NewRequest("POST", url, ioutil.NopCloser(bytes.NewReader([]byte(`{}`))))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.Header.Add(evergreen.HostHeader, hostId)
	request.Header.Add(evergreen.HostSecretHeader, hostSecret)
	request.Header.Add(evergreen.TaskSecretHeader, taskSecret)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
}

func TestAssignNextAvailableTaskWithPlannerSettingVersionLegacy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("with a task queue and a host", t, func() {
		settings := distro.PlannerSettings{
			Version: evergreen.PlannerVersionLegacy,
		}
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection, model.ProjectRefCollection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}
		distroID := "testDistro"
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
				Id:              distroID,
				PlannerSettings: settings,
			},
			Secret: hostSecret,
			Status: evergreen.HostRunning,
		}
		So(theHostWhoCanBoastTheMostRoast.Insert(), ShouldBeNil)

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
			Identifier: "exists",
			Enabled:    true,
		}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(pref.Insert(), ShouldBeNil)

		Convey("a host should get the task at the top of the queue", func() {
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "task1")

			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 1)

			h, err := host.FindOne(host.ById(theHostWhoCanBoastTheMostRoast.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, "task1")
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
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
			So(t.Id, ShouldEqual, "task2")

			currentTq, err := model.LoadTaskQueue(distroID)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 0)
		})
		Convey("an empty task queue should return a nil task", func() {
			taskQueue.Queue = []model.TaskQueueItem{}
			So(taskQueue.Save(), ShouldBeNil)
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
		})
		Convey("a tasks queue with a task that does not exist should continue", func() {
			taskQueue.Queue = []model.TaskQueueItem{{Id: "notatask"}}
			So(taskQueue.Save(), ShouldBeNil)
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
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
			So(anotherHost.Insert(), ShouldBeNil)
			h2 := host.Host{
				Id: "host2",
				Distro: distro.Distro{
					Id: distroID,
				},
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			So(h2.Insert(), ShouldBeNil)

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
				t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &h2)
				So(err, ShouldBeNil)
				So(t, ShouldNotBeNil)
				So(t.Id, ShouldEqual, t2.Id)
				h, err := host.FindOne(host.ById(h2.Id))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, t2.Id)
			})
			Convey("a host with a running task should return an error", func() {
				_, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &anotherHost)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestAssignNextAvailableTaskWithPlannerSettingVersionTunable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("with a task queue and a host", t, func() {
		settings := distro.PlannerSettings{
			Version: evergreen.PlannerVersionTunable,
		}
		if err := db.ClearCollections(distro.Collection, host.Collection, task.Collection, model.TaskQueuesCollection, model.ProjectRefCollection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}

		d := distro.Distro{
			Id:              "testDistro",
			PlannerSettings: settings,
		}

		So(d.Insert(), ShouldBeNil)

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
				Id:              d.Id,
				PlannerSettings: settings,
			},
			Secret: hostSecret,
			Status: evergreen.HostRunning,
		}
		So(theHostWhoCanBoastTheMostRoast.Insert(), ShouldBeNil)

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
			Identifier: "exists",
			Enabled:    true,
		}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(pref.Insert(), ShouldBeNil)

		Convey("a host should get the task at the top of the queue", func() {
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "task1")

			currentTq, err := model.LoadTaskQueue(d.Id)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 1)

			h, err := host.FindOne(host.ById(theHostWhoCanBoastTheMostRoast.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, "task1")
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
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
			So(t.Id, ShouldEqual, "task2")

			currentTq, err := model.LoadTaskQueue(d.Id)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 0)
		})
		Convey("an empty task queue should return a nil task", func() {
			taskQueue.Queue = []model.TaskQueueItem{}
			So(taskQueue.Save(), ShouldBeNil)
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
			So(t, ShouldBeNil)
		})
		Convey("a tasks queue with a task that does not exist should error", func() {
			item := model.TaskQueueItem{
				Id:           "notatask",
				Dependencies: []string{},
			}
			// taskQueue.Queue = []model.TaskQueueItem{{Id: "notatask"}}
			taskQueue.Queue = []model.TaskQueueItem{item}
			So(taskQueue.Save(), ShouldBeNil)
			t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &theHostWhoCanBoastTheMostRoast)
			So(err, ShouldBeNil)
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
			So(anotherHost.Insert(), ShouldBeNil)
			h2 := host.Host{
				Id: "host2",
				Distro: distro.Distro{
					Id: d.Id,
				},
				Secret: hostSecret,
				Status: evergreen.HostRunning,
			}
			So(h2.Insert(), ShouldBeNil)

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
				t, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &h2)
				So(err, ShouldBeNil)
				So(t, ShouldNotBeNil)
				So(t.Id, ShouldEqual, t2.Id)
				h, err := host.FindOne(host.ById(h2.Id))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, t2.Id)
			})
			Convey("a host with a running task should return an error", func() {
				_, err := assignNextAvailableTask(ctx, taskQueue, model.NewTaskDispatchService(taskDispatcherTTL), &anotherHost)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestNextTask(t *testing.T) {
	conf := testutil.TestConfig()
	queue := evergreen.GetEnvironment().LocalQueue()
	remoteQueue := evergreen.GetEnvironment().RemoteQueueGroup()

	Convey("with tasks, a host, a build, and a task queue", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection, evergreen.ConfigCollection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}
		if err := evergreen.SetServiceFlags(evergreen.ServiceFlags{}); err != nil {
			t.Fatalf("unable to create admin settings: %v", err)
		}

		as, err := NewAPIServer(conf, queue, remoteQueue)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}

		distroID := "testDistro"
		buildID := "buildId"

		tq := &model.TaskQueue{
			Distro: distroID,
			Queue: []model.TaskQueueItem{
				{Id: "task1"},
				{Id: "task2"},
			},
		}
		So(tq.Save(), ShouldBeNil)
		sampleHost := host.Host{
			Id: "h1",
			Distro: distro.Distro{
				Id: distroID,
			},
			Secret:        hostSecret,
			Status:        evergreen.HostRunning,
			AgentRevision: evergreen.BuildRevision,
		}
		So(sampleHost.Insert(), ShouldBeNil)

		task1 := task.Task{
			Id:        "task1",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			BuildId:   buildID,
			Project:   "exists",
		}
		So(task1.Insert(), ShouldBeNil)

		task2 := task.Task{
			Id:        "task2",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
			BuildId:   buildID,
		}
		So(task2.Insert(), ShouldBeNil)

		testBuild := build.Build{
			Id: buildID,
			Tasks: []build.TaskCache{
				{Id: "task1"},
				{Id: "task2"},
			},
		}
		So(testBuild.Insert(), ShouldBeNil)

		task3 := task.Task{
			Id:        "another",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
		}
		So(task3.Insert(), ShouldBeNil)

		pref := &model.ProjectRef{
			Identifier: "exists",
			Enabled:    true,
		}

		So(pref.Insert(), ShouldBeNil)

		sent := &apimodels.GetNextTaskDetails{}
		Convey("getting the next task api endpoint should work", func() {
			resp := getNextTaskEndpoint(t, as, sampleHost.Id, sent)
			So(resp, ShouldNotBeNil)
			Convey("should return an http status ok", func() {
				So(resp.Code, ShouldEqual, http.StatusOK)
				Convey("and a task with id task1", func() {
					taskResp := apimodels.NextTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.TaskId, ShouldEqual, "task1")
					nextTask, err := task.FindOne(task.ById(taskResp.TaskId))
					So(err, ShouldBeNil)
					So(nextTask.Status, ShouldEqual, evergreen.TaskDispatched)
				})
			})
			Convey("and should set the agent start time", func() {
				dbHost, err := host.FindOneId(sampleHost.Id)
				require.NoError(t, err)
				So(util.IsZeroTime(dbHost.AgentStartTime), ShouldBeFalse)
			})
			Convey("with degraded mode set", func() {
				serviceFlags := evergreen.ServiceFlags{
					TaskDispatchDisabled: true,
				}
				So(evergreen.SetServiceFlags(serviceFlags), ShouldBeNil)
				resp = getNextTaskEndpoint(t, as, sampleHost.Id, sent)
				So(resp, ShouldNotBeNil)
				Convey("then the response should not contain a task", func() {
					So(resp.Code, ShouldEqual, http.StatusOK)
					taskResp := apimodels.NextTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.TaskId, ShouldEqual, "")
					So(taskResp.ShouldExit, ShouldEqual, false)
				})
				serviceFlags.TaskDispatchDisabled = false // unset degraded mode
				So(evergreen.SetServiceFlags(serviceFlags), ShouldBeNil)
			})
			Convey("with an out of date agent revision and no task group", func() {
				So(sampleHost.SetAgentRevision("out-of-date-string"), ShouldBeNil)
				resp := getNextTaskEndpoint(t, as, sampleHost.Id, sent)
				details := &apimodels.NextTaskResponse{}
				So(resp.Code, ShouldEqual, http.StatusOK)
				So(json.NewDecoder(resp.Body).Decode(details), ShouldBeNil)
				So(details.ShouldExit, ShouldEqual, true)
				So(sampleHost.SetAgentRevision(evergreen.BuildRevision), ShouldBeNil) // reset
			})
			Convey("with an out of date agent revision and a task group", func() {
				So(sampleHost.SetAgentRevision("out-of-date-string"), ShouldBeNil)
				sentWithTaskGroup := &apimodels.GetNextTaskDetails{TaskGroup: "task_group"}
				resp := getNextTaskEndpoint(t, as, sampleHost.Id, sentWithTaskGroup)
				details := &apimodels.NextTaskResponse{}
				So(resp.Code, ShouldEqual, http.StatusOK)
				So(json.NewDecoder(resp.Body).Decode(details), ShouldBeNil)
				So(details.ShouldExit, ShouldEqual, false)
				So(sampleHost.SetAgentRevision(evergreen.BuildRevision), ShouldBeNil) // reset
			})
			Convey("with a non-legacy host with an old agent revision in the database", func() {
				nonLegacyHost := host.Host{
					Id: "nonLegacyHost",
					Distro: distro.Distro{
						Id: distroID,
						BootstrapSettings: distro.BootstrapSettings{

							Method:        distro.BootstrapMethodUserData,
							Communication: distro.CommunicationMethodRPC,
						},
					},
					Secret:        hostSecret,
					Status:        evergreen.HostRunning,
					AgentRevision: "out-of-date",
				}
				So(nonLegacyHost.Insert(), ShouldBeNil)

				Convey("with the latest agent revision in the next task details", func() {
					reqDetails := &apimodels.GetNextTaskDetails{AgentRevision: evergreen.BuildRevision}
					resp := getNextTaskEndpoint(t, as, nonLegacyHost.Id, reqDetails)
					So(resp.Code, ShouldEqual, http.StatusOK)
					respDetails := &apimodels.NextTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(respDetails), ShouldBeNil)
					So(respDetails.ShouldExit, ShouldBeFalse)
				})
				Convey("with an outdated agent revision in the next task details", func() {
					reqDetails := &apimodels.GetNextTaskDetails{AgentRevision: "out-of-date"}
					resp := getNextTaskEndpoint(t, as, nonLegacyHost.Id, reqDetails)
					So(resp.Code, ShouldEqual, http.StatusOK)
					respDetails := &apimodels.NextTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(respDetails), ShouldBeNil)
					So(respDetails.ShouldExit, ShouldBeTrue)
				})
			})
			Convey("with a host that already has a running task", func() {
				h2 := host.Host{
					Id:            "anotherHost",
					Secret:        hostSecret,
					RunningTask:   "existingTask",
					AgentRevision: evergreen.BuildRevision,
					Status:        evergreen.HostRunning,
				}
				So(h2.Insert(), ShouldBeNil)

				existingTask := task.Task{
					Id:        "existingTask",
					Status:    evergreen.TaskDispatched,
					Activated: true,
				}
				So(existingTask.Insert(), ShouldBeNil)
				Convey("getting the next task should return the existing task", func() {
					resp := getNextTaskEndpoint(t, as, h2.Id, sent)
					So(resp, ShouldNotBeNil)
					Convey("should return http status ok", func() {
						So(resp.Code, ShouldEqual, http.StatusOK)
						Convey("and a task with the existing task id", func() {
							taskResp := apimodels.NextTaskResponse{}
							So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
							So(taskResp.TaskId, ShouldEqual, "existingTask")
							nextTask, err := task.FindOne(task.ById(taskResp.TaskId))
							So(err, ShouldBeNil)
							So(nextTask.Status, ShouldEqual, evergreen.TaskDispatched)
						})
					})
				})
				Convey("with an undispatched task but a host that has that task as running task", func() {
					t1 := task.Task{
						Id:        "t1",
						Status:    evergreen.TaskUndispatched,
						Activated: true,
						BuildId:   "anotherBuild",
					}
					So(t1.Insert(), ShouldBeNil)
					anotherHost := host.Host{
						Id:            "sampleHost",
						Secret:        hostSecret,
						RunningTask:   t1.Id,
						AgentRevision: evergreen.BuildRevision,
						Status:        evergreen.HostRunning,
					}
					anotherBuild := build.Build{
						Id: "anotherBuild",
						Tasks: []build.TaskCache{
							{Id: t1.Id},
						},
					}

					So(anotherBuild.Insert(), ShouldBeNil)
					So(anotherHost.Insert(), ShouldBeNil)
					Convey("t1 should be returned and should be set to dispatched", func() {
						resp := getNextTaskEndpoint(t, as, anotherHost.Id, sent)
						So(resp, ShouldNotBeNil)
						Convey("should return http status ok", func() {
							So(resp.Code, ShouldEqual, http.StatusOK)
							Convey("task should exist with the existing task id and be dispatched", func() {
								taskResp := apimodels.NextTaskResponse{}
								So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
								So(taskResp.TaskId, ShouldEqual, t1.Id)
								nextTask, err := task.FindOne(task.ById(taskResp.TaskId))
								So(err, ShouldBeNil)
								So(nextTask.Status, ShouldEqual, evergreen.TaskDispatched)
							})
						})
					})
					Convey("an inactive task should not be dispatched even if it's assigned to a host", func() {
						inactiveTask := task.Task{
							Id:        "t2",
							Status:    evergreen.TaskUndispatched,
							Activated: false,
							BuildId:   "anotherBuild",
						}
						So(inactiveTask.Insert(), ShouldBeNil)
						h3 := host.Host{
							Id:            "inactive",
							Secret:        hostSecret,
							RunningTask:   inactiveTask.Id,
							Status:        evergreen.HostRunning,
							AgentRevision: evergreen.BuildRevision,
						}
						So(h3.Insert(), ShouldBeNil)
						anotherBuild := build.Build{
							Id: "b",
							Tasks: []build.TaskCache{
								{Id: inactiveTask.Id},
							},
						}
						So(anotherBuild.Insert(), ShouldBeNil)
						Convey("the inactive task should not be returned and the host running task should be unset", func() {
							resp := getNextTaskEndpoint(t, as, h3.Id, sent)
							So(resp, ShouldNotBeNil)
							Convey("should return http status ok", func() {
								So(resp.Code, ShouldEqual, http.StatusOK)
								Convey("task should exist with the existing task id and be dispatched", func() {
									taskResp := apimodels.NextTaskResponse{}
									So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
									So(taskResp.TaskId, ShouldEqual, "")
									h, err := host.FindOne(host.ById(h3.Id))
									So(err, ShouldBeNil)
									So(h.RunningTask, ShouldEqual, "")
								})
							})
						})
					})
				})
			})
		})
	})
}

func TestValidateTaskEndDetails(t *testing.T) {
	Convey("With a set of end details with different statuses", t, func() {
		details := apimodels.TaskEndDetail{}
		details.Status = evergreen.TaskUndispatched
		So(validateTaskEndDetails(&details), ShouldBeTrue)
		details.Status = evergreen.TaskDispatched
		So(validateTaskEndDetails(&details), ShouldBeFalse)
		details.Status = evergreen.TaskFailed
		So(validateTaskEndDetails(&details), ShouldBeTrue)
		details.Status = evergreen.TaskSucceeded
		So(validateTaskEndDetails(&details), ShouldBeTrue)
	})
}

func TestCheckHostHealth(t *testing.T) {
	currentRevision := "abc"
	Convey("With a host that has different statuses", t, func() {
		h := &host.Host{
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

func localGroupConstructor(ctx context.Context) (amboy.Queue, error) {
	return queue.NewLocalLimitedSize(1, 1048), nil
}

func TestTaskLifecycleEndpoints(t *testing.T) {
	conf := testutil.TestConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("with tasks, a host, a build, and a task queue", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection,
			build.Collection, model.ProjectRefCollection, model.VersionCollection, alertrecord.Collection, event.AllLogCollection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}

		q := queue.NewLocalLimitedSize(4, 2048)
		if err := q.Start(ctx); err != nil {
			t.Fatalf("failed to start queue %s", err)
		}
		opts := queue.LocalQueueGroupOptions{Constructor: localGroupConstructor}
		group, err := queue.NewLocalQueueGroup(ctx, opts)
		So(err, ShouldBeNil)

		as, err := NewAPIServer(conf, q, group)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		So(as.queue, ShouldEqual, q)

		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}

		hostId := "h1"
		projectId := "proj"
		buildID := "b1"
		versionId := "v1"

		proj := model.ProjectRef{
			Identifier: projectId,
		}
		So(proj.Insert(), ShouldBeNil)

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
		So(task1.Insert(), ShouldBeNil)

		sampleHost := host.Host{
			Id: hostId,
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2Auto,
			},
			Secret:                hostSecret,
			RunningTask:           task1.Id,
			Provider:              evergreen.ProviderNameStatic,
			Status:                evergreen.HostRunning,
			AgentRevision:         evergreen.BuildRevision,
			LastTaskCompletedTime: time.Now().Add(-20 * time.Minute).Round(time.Second),
		}
		So(sampleHost.Insert(), ShouldBeNil)

		testBuild := build.Build{
			Id: buildID,
			Tasks: []build.TaskCache{
				{Id: "task1"},
				{Id: "task2"},
				{Id: "dt"},
			},
			Project: projectId,
			Version: versionId,
		}
		So(testBuild.Insert(), ShouldBeNil)

		testVersion := model.Version{
			Id:     versionId,
			Branch: projectId,
		}
		So(testVersion.Insert(), ShouldBeNil)

		Convey("test task should start a background job", func() {
			stat := q.Stats(ctx)
			So(stat.Total, ShouldEqual, 0)
			resp := getStartTaskEndpoint(t, as, hostId, task1.Id)
			stat = q.Stats(ctx)

			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp, ShouldNotBeNil)
			So(stat.Total, ShouldEqual, 1)
			amboy.WaitInterval(ctx, q, time.Millisecond)

			counter := 0
			for job := range as.queue.Results(ctx) {
				So(job, ShouldNotBeNil)

				switch job.Type().Name {
				case "collect-host-idle-data":
					counter++

					t := job.TimeInfo()
					So(t.Start.Before(t.End), ShouldBeTrue)
					So(t.Start.IsZero(), ShouldBeFalse)
					So(t.End.IsZero(), ShouldBeFalse)

				case "collect-task-start-data":
					counter++
				default:
					counter--
				}
			}

			So(counter, ShouldEqual, stat.Total)

		})
		Convey("with a set of task end details indicating that task has succeeded", func() {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
			}
			resp := getEndTaskEndpoint(t, as, hostId, task1.Id, details)
			So(resp, ShouldNotBeNil)
			Convey("should return http status ok", func() {
				So(resp.Code, ShouldEqual, http.StatusOK)
				Convey("task should exist with the existing task id and be dispatched", func() {
					taskResp := apimodels.EndTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.ShouldExit, ShouldBeFalse)
				})
			})
			Convey("the host should no longer have the task set as its running task", func() {
				h, err := host.FindOne(host.ById(hostId))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, "")
				Convey("the task should be marked as succeeded and the task end details"+
					"should be added to the task document", func() {
					t, err := task.FindOne(task.ById(task1.Id))
					So(err, ShouldBeNil)
					So(t.Status, ShouldEqual, evergreen.TaskSucceeded)
					So(t.Details.Status, ShouldEqual, evergreen.TaskSucceeded)
				})
			})
		})
		Convey("with a set of task end details indicating that task has failed", func() {
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			testTask, err := task.FindOne(task.ById(task1.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
			resp := getEndTaskEndpoint(t, as, hostId, task1.Id, details)
			So(resp, ShouldNotBeNil)
			Convey("should return http status ok", func() {
				So(resp.Code, ShouldEqual, http.StatusOK)
				Convey("task should exist with the existing task id and be dispatched", func() {
					taskResp := apimodels.EndTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.ShouldExit, ShouldBeFalse)
				})
			})
			Convey("the host should no longer have the task set as its running task", func() {
				h, err := host.FindOne(host.ById(hostId))
				So(err, ShouldBeNil)
				So(h.RunningTask, ShouldEqual, "")
				Convey("the task should be marked as succeeded and the task end details"+
					"should be added to the task document", func() {
					t, err := task.FindOne(task.ById(task1.Id))
					So(err, ShouldBeNil)
					So(t.Status, ShouldEqual, evergreen.TaskFailed)
					So(t.Details.Status, ShouldEqual, evergreen.TaskFailed)
				})
			})
		})
		Convey("with a set of task end details but a task that is inactive", func() {
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
			So(task2.Insert(), ShouldBeNil)

			sampleHost := host.Host{
				Id:            "h2",
				Secret:        hostSecret,
				RunningTask:   task2.Id,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.BuildRevision,
			}
			So(sampleHost.Insert(), ShouldBeNil)

			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskUndispatched,
			}
			testTask, err := task.FindOne(task.ById(task1.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
			resp := getEndTaskEndpoint(t, as, sampleHost.Id, task2.Id, details)
			So(resp, ShouldNotBeNil)
			Convey("should return http status ok", func() {
				So(resp.Code, ShouldEqual, http.StatusOK)
				Convey("task should exist with the existing task id and be dispatched", func() {
					taskResp := apimodels.EndTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.ShouldExit, ShouldBeFalse)
				})
			})
		})

		Convey("with a display task", func() {
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
			So(execTask.Insert(), ShouldBeNil)
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
			So(displayTask.Insert(), ShouldBeNil)

			sampleHost := host.Host{
				Id:            "h2",
				Secret:        hostSecret,
				RunningTask:   execTask.Id,
				Status:        evergreen.HostRunning,
				AgentRevision: evergreen.BuildRevision,
			}
			So(sampleHost.Insert(), ShouldBeNil)

			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			resp := getEndTaskEndpoint(t, as, sampleHost.Id, execTask.Id, details)
			So(resp, ShouldNotBeNil)
			Convey("should return http status ok", func() {
				So(resp.Code, ShouldEqual, http.StatusOK)
				Convey("task should exist with the existing task id and be dispatched", func() {
					taskResp := apimodels.EndTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.ShouldExit, ShouldBeFalse)
				})
			})
			Convey("the display task should be updated correctly", func() {
				dbTask, err := task.FindOne(task.ById(displayTask.Id))
				So(err, ShouldBeNil)
				So(dbTask.Status, ShouldEqual, evergreen.TaskFailed)
			})
			Convey("the build cache should be updated correctly", func() {
				dbBuild, err := build.FindOne(build.ById(buildID))
				So(err, ShouldBeNil)
				So(dbBuild.Tasks[2].Status, ShouldEqual, evergreen.TaskFailed)
			})
		})
	})
}
