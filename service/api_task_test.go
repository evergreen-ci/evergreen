package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/taskrunner"
	"github.com/evergreen-ci/evergreen/testutil"
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

func getNextTaskEndpoint(t *testing.T, as *APIServer, hostId string) *httptest.ResponseRecorder {
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

func TestAssignNextAvailableTask(t *testing.T) {
	Convey("with a task queue and a host", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection); err != nil {
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
		}
		So(task1.Insert(), ShouldBeNil)

		task2 := task.Task{
			Id:        "task2",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
		}
		So(task2.Insert(), ShouldBeNil)
		Convey("a host should get the task at the top of the queue", func() {
			t, err := assignNextAvailableTask(tq, &sampleHost)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "task1")

			currentTq, err := model.FindTaskQueueForDistro(distroId)
			So(err, ShouldBeNil)
			So(len(currentTq.Queue), ShouldEqual, 1)

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

				currentTq, err := model.FindTaskQueueForDistro(distroId)
				So(err, ShouldBeNil)
				So(len(currentTq.Queue), ShouldEqual, 0)
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
					Activated: true,
				}
				So(t1.Insert(), ShouldBeNil)
				t2 := task.Task{
					Id:        "another",
					Status:    evergreen.TaskUndispatched,
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
	Convey("with tasks, a host, a build, and a task queue", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection, admin.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		if err := modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey); err != nil {
			t.Fatalf("adding test indexes %v", err)
		}
		if err := admin.Upsert(&admin.AdminSettings{}); err != nil {
			t.Fatalf("unable to create admin settings: %v", err)
		}

		as, err := NewAPIServer(testutil.TestConfig())
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		taskRunnerInstance := taskrunner.NewTaskRunner(&as.Settings)
		agentRevision, err := taskRunnerInstance.HostGateway.GetAgentRevision()
		So(err, ShouldBeNil)

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
			AgentRevision: agentRevision,
		}
		So(sampleHost.Insert(), ShouldBeNil)

		task1 := task.Task{
			Id:        "task1",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			BuildId:   buildId,
		}
		So(task1.Insert(), ShouldBeNil)

		task2 := task.Task{
			Id:        "task2",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
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
		Convey("getting the next task api endpoint should work", func() {
			resp := getNextTaskEndpoint(t, as, sampleHost.Id)
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
				serviceFlags := admin.ServiceFlags{
					TaskDispatchDisabled: true,
				}
				So(admin.SetServiceFlags(serviceFlags), ShouldBeNil)
				resp = getNextTaskEndpoint(t, as, sampleHost.Id)
				So(resp, ShouldNotBeNil)
				Convey("then the response should not contain a task", func() {
					So(resp.Code, ShouldEqual, http.StatusOK)
					taskResp := apimodels.NextTaskResponse{}
					So(json.NewDecoder(resp.Body).Decode(&taskResp), ShouldBeNil)
					So(taskResp.TaskId, ShouldEqual, "")
					So(taskResp.ShouldExit, ShouldEqual, false)
				})
				serviceFlags.TaskDispatchDisabled = false // unset degraded mode
				So(admin.SetServiceFlags(serviceFlags), ShouldBeNil)
			})
			Convey("with a host that already has a running task", func() {
				h2 := host.Host{
					Id:            "anotherHost",
					Secret:        hostSecret,
					RunningTask:   "existingTask",
					AgentRevision: agentRevision,
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
					resp := getNextTaskEndpoint(t, as, h2.Id)
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
						AgentRevision: agentRevision,
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
						resp := getNextTaskEndpoint(t, as, anotherHost.Id)
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
							AgentRevision: agentRevision,
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
							resp := getNextTaskEndpoint(t, as, h3.Id)
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
		shouldExit, _ := checkHostHealth(h, currentRevision)
		So(shouldExit, ShouldBeFalse)
		h.Status = evergreen.HostDecommissioned
		shouldExit, _ = checkHostHealth(h, currentRevision)
		So(shouldExit, ShouldBeTrue)
		h.Status = evergreen.HostQuarantined
		shouldExit, _ = checkHostHealth(h, currentRevision)
		So(shouldExit, ShouldBeTrue)
		Convey("With a host that is running but has a different revision", func() {
			shouldExit, _ := checkHostHealth(h, "bcd")
			So(shouldExit, ShouldBeTrue)
		})
	})
}

func TestEndTaskEndpoint(t *testing.T) {
	Convey("with tasks, a host, a build, and a task queue", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection, model.TaskQueuesCollection,
			build.Collection, model.ProjectRefCollection, version.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		as, err := NewAPIServer(testutil.TestConfig())
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		taskRunnerInstance := taskrunner.NewTaskRunner(&as.Settings)
		agentRevision, err := taskRunnerInstance.HostGateway.GetAgentRevision()
		So(err, ShouldBeNil)

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
			Id:            hostId,
			Secret:        hostSecret,
			RunningTask:   task1.Id,
			Status:        evergreen.HostRunning,
			AgentRevision: agentRevision,
		}
		So(sampleHost.Insert(), ShouldBeNil)

		testBuild := build.Build{
			Id: buildId,
			Tasks: []build.TaskCache{
				{Id: "task1"},
				{Id: "task2"},
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
				AgentRevision: agentRevision,
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
					So(taskResp.ShouldExit, ShouldBeTrue)
				})
			})
		})

	})
}
