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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	hostSecret = "secret"
	taskSecret = "tasksecret"
)

func insertHostWithRunningTask(hostId, taskId string) (host.Host, error) {
	h := host.Host{
		Id:          hostId,
		RunningTask: taskId,
	}
	return h, h.Insert()
}

func getNextTaskEndpoint(t *testing.T, as *APIServer, hostId string, details *apimodels.GetNextTaskDetails) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.Handler()
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
	testutil.HandleTestingErr(err, t, "error marshalling json")
	request.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
}

func getEndTaskEndpoint(t *testing.T, as *APIServer, hostId, taskId string, details *apimodels.TaskEndDetail) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := fmt.Sprintf("/api/2/task/%v/new_end", taskId)

	request, err := http.NewRequest("POST", url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.Header.Add(evergreen.HostHeader, hostId)
	request.Header.Add(evergreen.HostSecretHeader, hostSecret)
	request.Header.Add(evergreen.TaskSecretHeader, taskSecret)

	jsonBytes, err := json.Marshal(*details)
	testutil.HandleTestingErr(err, t, "error marshalling json")
	request.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
}

func getStartTaskEndpoint(t *testing.T, as *APIServer, hostId, taskId string) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.Handler()
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

func TestAssignNextAvailableTask(t *testing.T) {
	Convey("with a task queue and a host", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection, model.ProjectRefCollection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}
		distroId := "testDistro"

		tq := &model.TaskQueue{
			Distro: distroId,
			Queue: []model.TaskQueueItem{
				{Id: "task1"},
				{Id: "task2"},
			},
		}
		So(tq.Save(), ShouldBeNil)
		sampleHost := host.Host{
			Id: "h1",
			Distro: distro.Distro{
				Id: distroId,
			},
			Secret: hostSecret,
		}
		So(sampleHost.Insert(), ShouldBeNil)

		task1 := task.Task{
			Id:        "task1",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
		}
		So(task1.Insert(), ShouldBeNil)

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

		So(pref.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		Convey("a host should get the task at the top of the queue", func() {
			t, err := assignNextAvailableTask(tq, &sampleHost)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "task1")

			currentTq, err := model.LoadTaskQueue(distroId)
			So(err, ShouldBeNil)
			So(currentTq.Length(), ShouldEqual, 1)

			h, err := host.FindOne(host.ById(sampleHost.Id))
			So(err, ShouldBeNil)
			So(h.RunningTask, ShouldEqual, "task1")

			Convey("a task that is not undispatched should not be updated in the host", func() {
				tq.Queue = []model.TaskQueueItem{
					{Id: "undispatchedTask"},
					{Id: "task2"},
				}
				So(tq.Save(), ShouldBeNil)
				undispatchedTask := task.Task{
					Id:     "undispatchedTask",
					Status: evergreen.TaskStarted,
				}
				So(undispatchedTask.Insert(), ShouldBeNil)
				t, err := assignNextAvailableTask(tq, &sampleHost)
				So(err, ShouldBeNil)
				So(t.Id, ShouldEqual, "task2")

				currentTq, err := model.LoadTaskQueue(distroId)
				So(err, ShouldBeNil)
				So(currentTq.Length(), ShouldEqual, 0)
			})
			Convey("an empty task queue should return a nil task", func() {
				tq.Queue = []model.TaskQueueItem{}
				So(tq.Save(), ShouldBeNil)
				t, err := assignNextAvailableTask(tq, &sampleHost)
				So(err, ShouldBeNil)
				So(t, ShouldBeNil)
			})
			Convey("a tasks queue with a task that does not exist should error", func() {
				tq.Queue = []model.TaskQueueItem{{Id: "notatask"}}
				So(tq.Save(), ShouldBeNil)
				_, err := assignNextAvailableTask(tq, h)
				So(err, ShouldNotBeNil)
			})
			Convey("with a host with a running task", func() {
				anotherHost := host.Host{
					Id:          "ahost",
					RunningTask: "sampleTask",
					Distro: distro.Distro{
						Id: distroId,
					},
					Secret: hostSecret,
				}
				So(anotherHost.Insert(), ShouldBeNil)
				h2 := host.Host{
					Id: "host2",
					Distro: distro.Distro{
						Id: distroId,
					},
					Secret: hostSecret,
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

				tq.Queue = []model.TaskQueueItem{
					{Id: t1.Id},
					{Id: t2.Id},
				}
				So(tq.Save(), ShouldBeNil)
				Convey("the task that is in the other host should not be assigned to another host", func() {
					t, err := assignNextAvailableTask(tq, &h2)
					So(err, ShouldBeNil)
					So(t, ShouldNotBeNil)
					So(t.Id, ShouldEqual, t2.Id)
					h, err := host.FindOne(host.ById(h2.Id))
					So(err, ShouldBeNil)
					So(h.RunningTask, ShouldEqual, t2.Id)
				})
				Convey("a host with a running task should return an error", func() {
					_, err := assignNextAvailableTask(tq, &anotherHost)
					So(err, ShouldNotBeNil)
				})

			})
		})

	})
}

func TestNextTask(t *testing.T) {
	conf := testutil.TestConfig()
	queue := evergreen.GetEnvironment().LocalQueue()

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

		as, err := NewAPIServer(conf, queue)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}

		distroId := "testDistro"
		buildId := "buildId"

		tq := &model.TaskQueue{
			Distro: distroId,
			Queue: []model.TaskQueueItem{
				{Id: "task1"},
				{Id: "task2"},
			},
		}
		So(tq.Save(), ShouldBeNil)
		sampleHost := host.Host{
			Id: "h1",
			Distro: distro.Distro{
				Id: distroId,
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
			BuildId:   buildId,
			Project:   "exists",
		}
		So(task1.Insert(), ShouldBeNil)

		task2 := task.Task{
			Id:        "task2",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "exists",
			BuildId:   buildId,
		}
		So(task2.Insert(), ShouldBeNil)

		testBuild := build.Build{
			Id: buildId,
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
				So(details.NewAgent, ShouldEqual, false)
				So(sampleHost.SetAgentRevision(evergreen.BuildRevision), ShouldBeNil) // reset
			})
			Convey("with an out of date agent revision and a task group", func() {
				So(sampleHost.SetAgentRevision("out-of-date-string"), ShouldBeNil)
				sentWithTaskGroup := &apimodels.GetNextTaskDetails{"task_group"}
				resp := getNextTaskEndpoint(t, as, sampleHost.Id, sentWithTaskGroup)
				details := &apimodels.NextTaskResponse{}
				So(resp.Code, ShouldEqual, http.StatusOK)
				So(json.NewDecoder(resp.Body).Decode(details), ShouldBeNil)
				So(details.ShouldExit, ShouldEqual, false)
				So(details.NewAgent, ShouldEqual, true)
				So(sampleHost.SetAgentRevision(evergreen.BuildRevision), ShouldBeNil) // reset
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
			shouldExit := checkAgentRevision(h)
			So(shouldExit, ShouldBeTrue)
		})
	})
}

func TestTaskLifecycleEndpoints(t *testing.T) {
	conf := testutil.TestConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("with tasks, a host, a build, and a task queue", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection,
			build.Collection, model.ProjectRefCollection, version.Collection, alertrecord.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}

		q := queue.NewLocalLimitedSize(4, 2048)
		if err := q.Start(ctx); err != nil {
			t.Fatalf("failed to start queue %s", err)
		}

		as, err := NewAPIServer(conf, q)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		So(as.queue, ShouldEqual, q)

		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}

		hostId := "h1"
		projectId := "proj"
		buildId := "b1"
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
			BuildId:   buildId,
			Version:   versionId,
		}
		So(task1.Insert(), ShouldBeNil)

		sampleHost := host.Host{
			Id:                    hostId,
			Secret:                hostSecret,
			RunningTask:           task1.Id,
			Provider:              evergreen.ProviderNameStatic,
			Status:                evergreen.HostRunning,
			AgentRevision:         evergreen.BuildRevision,
			LastTaskCompletedTime: time.Now().Add(-20 * time.Minute).Round(time.Second),
		}
		So(sampleHost.Insert(), ShouldBeNil)

		testBuild := build.Build{
			Id: buildId,
			Tasks: []build.TaskCache{
				{Id: "task1"},
				{Id: "task2"},
				{Id: "dt"},
			},
			Project: projectId,
			Version: versionId,
		}
		So(testBuild.Insert(), ShouldBeNil)

		testVersion := version.Version{
			Id:     versionId,
			Branch: projectId,
		}
		So(testVersion.Insert(), ShouldBeNil)

		Convey("test task should start a background job", func() {
			stat := q.Stats()
			So(stat.Total, ShouldEqual, 0)
			resp := getStartTaskEndpoint(t, as, hostId, task1.Id)
			stat = q.Stats()

			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp, ShouldNotBeNil)
			So(stat.Total, ShouldEqual, 2)
			amboy.WaitCtxInterval(ctx, q, time.Millisecond)

			counter := 0
			for job := range as.queue.Results(ctx) {
				So(job, ShouldNotBeNil)

				switch job.Type().Name {
				case "collect-host-idle-data":
					counter++

					// this is gross, but lets us introspect the private job
					jobData := struct {
						StartAt  time.Time `json:"start_time"`
						FinishAt time.Time `json:"finish_time"`
					}{}

					raw, err := json.Marshal(job)
					So(err, ShouldBeNil)
					So(json.Unmarshal(raw, &jobData), ShouldBeNil)

					So(jobData.StartAt.Before(jobData.FinishAt), ShouldBeTrue)
					So(jobData.StartAt.IsZero(), ShouldBeFalse)
					So(jobData.FinishAt.IsZero(), ShouldBeFalse)

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
				BuildId:   buildId,
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
				BuildId:      buildId,
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
				BuildId:        buildId,
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
				dbBuild, err := build.FindOne(build.ById(buildId))
				So(err, ShouldBeNil)
				So(dbBuild.Tasks[2].Status, ShouldEqual, evergreen.TaskFailed)
			})
			Convey("alerts should be created for the build failure", func() {
				dbAlert, err := alertrecord.FindOne(alertrecord.ByLastFailureTransition(
					displayTask.DisplayName, displayTask.BuildVariant, displayTask.Project))
				So(err, ShouldBeNil)
				So(dbAlert.Type, ShouldEqual, alertrecord.TaskFailTransitionId)
				So(dbAlert.TaskId, ShouldEqual, displayTask.Id)

				// alerts should not have been created for the execution task
				execTaskAlert, err := alertrecord.FindOne(alertrecord.ByLastFailureTransition(
					execTask.DisplayName, execTask.BuildVariant, execTask.Project))
				So(err, ShouldBeNil)
				So(execTaskAlert, ShouldBeNil)
			})
		})
	})
}
