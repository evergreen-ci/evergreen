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
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
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
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	hostSecret = "secret"
	taskSecret = "tasksecret"
)

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

func getDownstreamParamsEndpoint(t *testing.T, as *APIServer, hostId, taskId string, details []patch.Parameter) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	handler, err := as.GetServiceApp().Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := fmt.Sprintf("/api/2/task/%s/downstreamParams", taskId)

	request, err := http.NewRequest("POST", url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.Header.Add(evergreen.HostHeader, hostId)
	request.Header.Add(evergreen.HostSecretHeader, hostSecret)
	request.Header.Add(evergreen.TaskSecretHeader, taskSecret)

	jsonBytes, err := json.Marshal(details)
	require.NoError(t, err, "error marshalling json")
	request.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
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

func TestTaskLifecycleEndpoints(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("with tasks, a host, a build, and a task queue", t, func() {
		colls := []string{host.Collection, task.Collection, model.TaskQueuesCollection, build.Collection, model.ParserProjectCollection, model.ProjectRefCollection, model.VersionCollection, alertrecord.Collection, event.LegacyEventLogCollection}
		if err := db.DropCollections(colls...); err != nil {
			t.Fatalf("dropping collections: %v", err)
		}
		defer func() {
			assert.NoError(t, db.DropCollections(colls...))
		}()

		q := queue.NewLocalLimitedSize(4, 2048)
		if err := q.Start(ctx); err != nil {
			t.Fatalf("failed to start queue %s", err)
		}

		as, err := NewAPIServer(env, q)
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
			Id: projectId,
		}
		parserProj := model.ParserProject{
			Id: versionId,
		}
		So(parserProj.Insert(), ShouldBeNil)
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
				Provider: evergreen.ProviderNameMock,
			},
			Secret:                hostSecret,
			RunningTask:           task1.Id,
			Provider:              evergreen.ProviderNameStatic,
			Status:                evergreen.HostRunning,
			AgentRevision:         evergreen.AgentVersion,
			LastTaskCompletedTime: time.Now().Add(-20 * time.Minute).Round(time.Second),
		}
		So(sampleHost.Insert(), ShouldBeNil)

		testBuild := build.Build{
			Id:      buildID,
			Project: projectId,
			Version: versionId,
		}
		So(testBuild.Insert(), ShouldBeNil)

		testVersion := model.Version{
			Id:     versionId,
			Branch: projectId,
			Config: "identifier: " + projectId,
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
					t, err := task.FindOne(db.Query(task.ById(task1.Id)))
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
			testTask, err := task.FindOne(db.Query(task.ById(task1.Id)))
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
					t, err := task.FindOne(db.Query(task.ById(task1.Id)))
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
				AgentRevision: evergreen.AgentVersion,
			}
			So(sampleHost.Insert(), ShouldBeNil)

			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskUndispatched,
			}
			testTask, err := task.FindOne(db.Query(task.ById(task1.Id)))
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
				AgentRevision: evergreen.AgentVersion,
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
				dbTask, err := task.FindOne(db.Query(task.ById(displayTask.Id)))
				So(err, ShouldBeNil)
				So(dbTask.Status, ShouldEqual, evergreen.TaskFailed)
			})
		})
	})
}

func TestDownstreamParams(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a patch with downstream params", t, func() {
		if err := db.ClearCollections(patch.Collection, task.Collection, host.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
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
		So(parentPatch.Insert(), ShouldBeNil)

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
		So(task1.Insert(), ShouldBeNil)

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
		So(sampleHost.Insert(), ShouldBeNil)

		q := queue.NewLocalLimitedSize(4, 2048)
		if err := q.Start(ctx); err != nil {
			t.Fatalf("failed to start queue %s", err)
		}

		as, err := NewAPIServer(env, q)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		So(as.queue, ShouldEqual, q)

		Convey("with a patchId and parameters set in PatchData", func() {

			resp := getDownstreamParamsEndpoint(t, as, hostId, task1.Id, parameters)
			So(resp, ShouldNotBeNil)
			Convey("should return http status ok", func() {
				So(resp.Code, ShouldEqual, http.StatusOK)
			})
			Convey("the patches should appear in the parent patch", func() {
				p, err := patch.FindOneId(parentPatchId)
				So(err, ShouldBeNil)
				So(p.Triggers.DownstreamParameters[0].Key, ShouldEqual, parameters[0].Key)
				So(p.Triggers.DownstreamParameters[0].Value, ShouldEqual, parameters[0].Value)
				So(p.Triggers.DownstreamParameters[1].Key, ShouldEqual, parameters[1].Key)
				So(p.Triggers.DownstreamParameters[1].Value, ShouldEqual, parameters[1].Value)
			})
		})
	})
}

func TestHandleEndTaskForCommitQueueTask(t *testing.T) {
	require.NoError(t, db.CreateCollections(task.OldCollection))
	p1 := mgobson.NewObjectId().Hex()
	p2 := mgobson.NewObjectId().Hex()
	p3 := mgobson.NewObjectId().Hex()
	taskA := task.Task{
		Id:           "taskA",
		Version:      p1,
		Project:      "my_project",
		DisplayName:  "important_task",
		BuildVariant: "best_variant",
	}
	taskB := task.Task{
		Id:           "taskB",
		Version:      p2,
		Project:      "my_project",
		DisplayName:  "important_task",
		BuildVariant: "best_variant",
	}
	taskC := task.Task{
		Id:           "taskC",
		Version:      p3,
		Project:      "my_project",
		DisplayName:  "important_task",
		BuildVariant: "best_variant",
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
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(&taskA, evergreen.TaskSucceeded))

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
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(&taskA, evergreen.TaskSucceeded))

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
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(&taskA, evergreen.TaskSucceeded))

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
			assert.NoError(t, cq.UpdateVersion(itemToChange))
			assert.Empty(t, cq.Queue[1].Version)

			taskA.Status = evergreen.TaskSucceeded
			assert.NoError(t, taskA.Insert())

			// shouldn't do anything since taskB isn't scheduled
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(&taskA, evergreen.TaskSucceeded))

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
			assert.NoError(t, model.HandleEndTaskForCommitQueueTask(&taskB, evergreen.TaskFailed))

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
				task.Collection, patch.Collection, task.OldCollection))
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
			}
			assert.NoError(t, mergeTask1.Insert())
			mergeTask2 := task.Task{
				Id:               "mergeB",
				Version:          p2,
				CommitQueueMerge: true,
			}
			assert.NoError(t, mergeTask2.Insert())
			mergeTask3 := task.Task{
				Id:               "mergeC",
				Version:          p3,
				CommitQueueMerge: true,
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
