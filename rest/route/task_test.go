package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for abort task route

type TaskAbortSuite struct {
	suite.Suite
}

func TestTaskAbortSuite(t *testing.T) {
	suite.Run(t, new(TaskAbortSuite))
}

func (s *TaskAbortSuite) SetupSuite() {
	s.NoError(db.ClearCollections(task.Collection, user.Collection, build.Collection, serviceModel.VersionCollection))
	tasks := []task.Task{
		{Id: "task1", Status: evergreen.TaskStarted, Activated: true, BuildId: "b1", Version: "v1"},
		{Id: "task2", Status: evergreen.TaskStarted, Activated: true, BuildId: "b1", Version: "v1"},
	}
	s.NoError((&build.Build{Id: "b1"}).Insert(s.T().Context()))
	s.NoError((&serviceModel.Version{Id: "v1"}).Insert(s.T().Context()))
	u := &user.DBUser{Id: "user1"}
	s.NoError(u.Insert(s.T().Context()))
	for _, t := range tasks {
		s.NoError(t.Insert(s.T().Context()))
	}
}

func (s *TaskAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeTaskAbortHandler()
	rm.(*taskAbortHandler).taskId = "task1"
	res := rm.Run(ctx)

	s.Equal(http.StatusOK, res.Status())

	s.NotNil(res)
	tasks, err := task.Find(ctx, task.ByIds([]string{"task1", "task2"}))
	s.NoError(err)
	s.Equal("user1", tasks[0].ActivatedBy)
	s.Equal("", tasks[1].ActivatedBy)
	t, ok := res.Data().(*model.APITask)
	s.True(ok)
	s.Equal(utility.ToStringPtr("task1"), t.Id)

	res = rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)
	tasks, err = task.Find(ctx, task.ByIds([]string{"task1", "task2"}))
	s.NoError(err)
	s.Equal("user1", tasks[0].AbortInfo.User)
	s.Equal("", tasks[1].AbortInfo.User)
	t, ok = (res.Data()).(*model.APITask)
	s.True(ok)
	s.Equal(utility.ToStringPtr("task1"), t.Id)
}

func TestFetchArtifacts(t *testing.T) {
	ctx := t.Context()

	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.ClearCollections(task.Collection, task.OldCollection, artifact.Collection))
	task1 := task.Task{
		Id:        "task1",
		Status:    evergreen.TaskSucceeded,
		Execution: 0,
	}
	assert.NoError(task1.Insert(t.Context()))
	assert.NoError(task1.Archive(ctx))
	entry := artifact.Entry{
		TaskId:          task1.Id,
		TaskDisplayName: "task",
		BuildId:         "b1",
		Execution:       1,
		Files: []artifact.File{
			{
				Name: "file1",
				Link: "l1",
			},
			{
				Name: "file2",
				Link: "l2",
			},
		},
	}
	assert.NoError(entry.Upsert(t.Context()))
	entry.Execution = 0
	assert.NoError(entry.Upsert(t.Context()))

	task2 := task.Task{
		Id:          "task2",
		Execution:   0,
		DisplayOnly: true,
		Status:      evergreen.TaskSucceeded,
	}
	assert.NoError(task2.Insert(t.Context()))
	assert.NoError(task2.Archive(ctx))

	taskGet := taskGetHandler{taskID: task1.Id}
	resp := taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	apiTask := resp.Data().(*model.APITask)
	assert.Len(apiTask.Artifacts, 2)
	assert.Empty(apiTask.PreviousExecutions)

	// fetch all
	taskGet.fetchAllExecutions = true
	resp = taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	apiTask = resp.Data().(*model.APITask)
	require.Len(apiTask.PreviousExecutions, 1)
	assert.NotZero(apiTask.PreviousExecutions[0])
	assert.NotEmpty(apiTask.PreviousExecutions[0].Artifacts)

	// fetchs a display task
	taskGet.taskID = "task2"
	taskGet.fetchAllExecutions = false
	resp = taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	apiTask = resp.Data().(*model.APITask)
	assert.Empty(apiTask.PreviousExecutions)

	// fetch all, tasks with display tasks
	taskGet.fetchAllExecutions = true
	resp = taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	apiTask = resp.Data().(*model.APITask)
	require.Len(apiTask.PreviousExecutions, 1)
	assert.NotZero(apiTask.PreviousExecutions[0])
}

func TestGetDisplayTask(t *testing.T) {
	for testName, testCase := range map[string]func(context.Context, *testing.T){
		"SucceedsWithTaskInDisplayTask": func(ctx context.Context, t *testing.T) {
			tsk := task.Task{Id: "task_id"}
			displayTask := task.Task{
				Id:             "id",
				DisplayName:    "display_task_name",
				ExecutionTasks: []string{tsk.Id},
			}
			require.NoError(t, displayTask.Insert(t.Context()))

			h := makeGetDisplayTaskHandler()
			rh, ok := h.(*displayTaskGetHandler)
			require.True(t, ok)
			rh.taskID = tsk.Id
			require.NoError(t, tsk.Insert(t.Context()))

			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			info, ok := resp.Data().(*apimodels.DisplayTaskInfo)
			require.True(t, ok)
			assert.Equal(t, displayTask.Id, info.ID)
			assert.Equal(t, displayTask.DisplayName, info.Name)
		},
		"FailsWithNonexistentTask": func(ctx context.Context, t *testing.T) {
			h := makeGetDisplayTaskHandler()
			rh, ok := h.(*displayTaskGetHandler)
			require.True(t, ok)
			rh.taskID = "nonexistent"

			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"ReturnsOkIfNotPartOfDisplayTask": func(ctx context.Context, t *testing.T) {
			tsk := task.Task{Id: "task_id"}
			h := makeGetDisplayTaskHandler()
			require.NoError(t, tsk.Insert(t.Context()))
			rh, ok := h.(*displayTaskGetHandler)
			require.True(t, ok)
			rh.taskID = tsk.Id

			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			info, ok := resp.Data().(*apimodels.DisplayTaskInfo)
			require.True(t, ok)
			assert.Zero(t, *info)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx := t.Context()

			require.NoError(t, db.ClearCollections(task.Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(task.Collection))
			}()

			testCase(ctx, t)
		})
	}

}

func TestGeneratedTasksGetHandler(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *generatedTasksGetHandler, generatorID string, generated []task.Task){
		"ReturnsGeneratedTasks": func(ctx context.Context, t *testing.T, rh *generatedTasksGetHandler, generatorID string, generated []task.Task) {
			for _, tsk := range generated {
				require.NoError(t, tsk.Insert(t.Context()))
			}
			rh.taskID = generatorID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			data := resp.Data()
			require.NotZero(t, data)
			taskInfos, ok := data.([]model.APIGeneratedTaskInfo)
			require.True(t, ok)

			require.Len(t, taskInfos, len(generated))
			for i := 0; i < len(generated); i++ {
				assert.Equal(t, generated[i].Id, taskInfos[i].TaskID)
				assert.Equal(t, generated[i].DisplayName, taskInfos[i].TaskName)
				assert.Equal(t, generated[i].BuildId, taskInfos[i].BuildID)
				assert.Equal(t, generated[i].BuildVariant, taskInfos[i].BuildVariant)
				assert.Equal(t, generated[i].BuildVariantDisplayName, taskInfos[i].BuildVariantDisplayName)
			}
		},
		"ReturnsErrorWithNoMatches": func(ctx context.Context, t *testing.T, rh *generatedTasksGetHandler, generatorID string, generated []task.Task) {
			rh.taskID = "nonexistent"

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()

			require.NoError(t, db.ClearCollections(task.Collection))

			const generatorID = "generator"
			generated := []task.Task{
				{
					Id:                      "generated_task0",
					GeneratedBy:             generatorID,
					BuildId:                 "build_id0",
					BuildVariant:            "build-variant0",
					BuildVariantDisplayName: "first build variant",
				},
				{
					Id:                      "generated_task1",
					GeneratedBy:             generatorID,
					BuildId:                 "build_id1",
					BuildVariant:            "build-variant1",
					BuildVariantDisplayName: "second build variant",
				},
			}

			rh, ok := makeGetGeneratedTasks().(*generatedTasksGetHandler)
			require.True(t, ok)

			tCase(ctx, t, rh, generatorID, generated)
		})
	}
}

func TestUpdateArtifactURLHandler(t *testing.T) {
	for name, test := range map[string]func(t *testing.T){
		"SuccessAndExecutionOverride": func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection, user.Collection))

			tsk := task.Task{Id: "t1", BuildId: "b1", DisplayName: "disp", Execution: 0}
			require.NoError(t, tsk.Insert(t.Context()))
			entry := artifact.Entry{
				TaskId:          tsk.Id,
				TaskDisplayName: tsk.DisplayName,
				BuildId:         tsk.BuildId,
				Execution:       0,
				Files: []artifact.File{
					{
						Name: "f1",
						Link: "http://old.com/a",
					},
				},
			}
			require.NoError(t, entry.Upsert(t.Context()))

			projCtx := serviceModel.Context{Task: &tsk}
			u := &user.DBUser{Id: "u1"}
			require.NoError(t, u.Insert(t.Context()))
			ctxWithUser := gimlet.AttachUser(ctx, u)
			ctxWithProj := context.WithValue(ctxWithUser, RequestContext, &projCtx)

			body := map[string]string{"artifact_name": "f1", "current_url": "http://old.com/a", "new_url": "https://new.com/a"}
			data, _ := json.Marshal(body)
			h := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			req, _ := http.NewRequest(http.MethodPatch, "/tasks/t1/artifacts/url", bytes.NewReader(data))
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "t1"})
			require.NoError(t, h.Parse(ctxWithProj, req))
			resp := h.Run(ctxWithProj)
			assert.Equal(t, http.StatusOK, resp.Status())
			apiTask, ok := resp.Data().(*model.APITask)
			require.True(t, ok)
			foundUpdated := false
			for _, f := range apiTask.Artifacts {
				if utility.FromStringPtr(f.Name) == "f1" && utility.FromStringPtr(f.Link) == "https://new.com/a" {
					foundUpdated = true
				}
			}
			assert.True(t, foundUpdated)

			bad := map[string]string{"artifact_name": "f1", "current_url": "https://new.com/a", "new_url": "notaurl"}
			badData, _ := json.Marshal(bad)
			hBad := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			badReq, _ := http.NewRequest(http.MethodPatch, "/tasks/t1/artifacts/url", bytes.NewReader(badData))
			badReq = gimlet.SetURLVars(badReq, map[string]string{"task_id": "t1"})
			err := hBad.Parse(ctxWithProj, badReq)
			assert.Error(t, err)

			entry1 := artifact.Entry{
				TaskId:          tsk.Id,
				TaskDisplayName: tsk.DisplayName,
				BuildId:         tsk.BuildId,
				Execution:       1,
				Files: []artifact.File{
					{
						Name: "f1",
						Link: "http://old.com/a1",
					},
				},
			}
			require.NoError(t, entry1.Upsert(t.Context()))
			require.NoError(t, db.Update(task.Collection, bson.M{"_id": tsk.Id}, bson.M{"$set": bson.M{"execution": 1}}))
			refreshed, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			projCtx.Task = refreshed
			ctxWithProj = context.WithValue(ctxWithUser, RequestContext, &projCtx)

			bodyExec := map[string]string{"artifact_name": "f1", "current_url": "http://old.com/a1", "new_url": "https://new.com/a1"}
			dataExec, _ := json.Marshal(bodyExec)
			hExec := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			reqExec, _ := http.NewRequest(http.MethodPatch, "/tasks/t1/artifacts/url?execution=1", bytes.NewReader(dataExec))
			reqExec = gimlet.SetURLVars(reqExec, map[string]string{"task_id": "t1"})
			require.NoError(t, hExec.Parse(ctxWithProj, reqExec))
			respExec := hExec.Run(ctxWithProj)
			assert.Equal(t, http.StatusOK, respExec.Status())
			apiTaskExec, ok := respExec.Data().(*model.APITask)
			require.True(t, ok)
			updated := false
			for _, f := range apiTaskExec.Artifacts {
				if utility.FromStringPtr(f.Name) == "f1" && utility.FromStringPtr(f.Link) == "https://new.com/a1" {
					updated = true
				}
			}
			assert.True(t, updated)
		},
		"NotFoundScenarios": func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection, user.Collection))

			tsk := task.Task{Id: "nf1", BuildId: "b1", DisplayName: "disp", Execution: 0}
			require.NoError(t, tsk.Insert(t.Context()))
			entry := artifact.Entry{
				TaskId:          tsk.Id,
				TaskDisplayName: tsk.DisplayName,
				BuildId:         tsk.BuildId,
				Execution:       0,
				Files: []artifact.File{
					{
						Name: "afile",
						Link: "http://old.example/x",
					},
				},
			}
			require.NoError(t, entry.Upsert(t.Context()))

			projCtx := serviceModel.Context{Task: &tsk}
			u := &user.DBUser{Id: "userNF"}
			require.NoError(t, u.Insert(t.Context()))
			ctxWithUser := gimlet.AttachUser(ctx, u)
			ctxWithProj := context.WithValue(ctxWithUser, RequestContext, &projCtx)

			bodyBadName := map[string]string{"artifact_name": "wrong", "current_url": "http://old.example/x", "new_url": "https://new.example/x"}
			dataBadName, _ := json.Marshal(bodyBadName)
			hBadName := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			reqBadName, _ := http.NewRequest(http.MethodPatch, "/tasks/nf1/artifacts/url", bytes.NewReader(dataBadName))
			reqBadName = gimlet.SetURLVars(reqBadName, map[string]string{"task_id": "nf1"})
			require.NoError(t, hBadName.Parse(ctxWithProj, reqBadName))
			respBadName := hBadName.Run(ctxWithProj)
			assert.Equal(t, http.StatusNotFound, respBadName.Status())

			bodyBadURL := map[string]string{"artifact_name": "afile", "current_url": "http://does-not-match", "new_url": "https://new.example/x"}
			dataBadURL, _ := json.Marshal(bodyBadURL)
			hBadURL := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			reqBadURL, _ := http.NewRequest(http.MethodPatch, "/tasks/nf1/artifacts/url", bytes.NewReader(dataBadURL))
			reqBadURL = gimlet.SetURLVars(reqBadURL, map[string]string{"task_id": "nf1"})
			require.NoError(t, hBadURL.Parse(ctxWithProj, reqBadURL))
			respBadURL := hBadURL.Run(ctxWithProj)
			assert.Equal(t, http.StatusNotFound, respBadURL.Status())
		},
		"DefaultsToLatestExecution": func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection, user.Collection))

			// Task with two executions; latest is 2
			tsk := task.Task{Id: "late1", BuildId: "b1", DisplayName: "disp", Execution: 2}
			require.NoError(t, tsk.Insert(t.Context()))
			// Execution 0
			entry0 := artifact.Entry{
				TaskId:          tsk.Id,
				TaskDisplayName: tsk.DisplayName,
				BuildId:         tsk.BuildId,
				Execution:       0,
				Files: []artifact.File{
					{
						Name: "afile",
						Link: "http://old.example/x0",
					},
				},
			}
			require.NoError(t, entry0.Upsert(t.Context()))
			// Execution 2 (latest)
			entry2 := artifact.Entry{
				TaskId:          tsk.Id,
				TaskDisplayName: tsk.DisplayName,
				BuildId:         tsk.BuildId,
				Execution:       2,
				Files: []artifact.File{
					{
						Name: "afile",
						Link: "http://old.example/x2",
					},
				},
			}
			require.NoError(t, entry2.Upsert(t.Context()))

			projCtx := serviceModel.Context{Task: &tsk}
			u := &user.DBUser{Id: "userLate"}
			require.NoError(t, u.Insert(t.Context()))
			ctxWithUser := gimlet.AttachUser(ctx, u)
			ctxWithProj := context.WithValue(ctxWithUser, RequestContext, &projCtx)

			body := map[string]string{"artifact_name": "afile", "current_url": "http://old.example/x2", "new_url": "https://new.example/x2"}
			data, _ := json.Marshal(body)
			h := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			req, _ := http.NewRequest(http.MethodPatch, "/tasks/late1/artifacts/url", bytes.NewReader(data))
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "late1"})
			require.NoError(t, h.Parse(ctxWithProj, req))
			resp := h.Run(ctxWithProj)
			assert.Equal(t, http.StatusOK, resp.Status())
			apiTask, ok := resp.Data().(*model.APITask)
			require.True(t, ok)
			// Ensure updated link is reflected.
			found := false
			for _, f := range apiTask.Artifacts {
				if utility.FromStringPtr(f.Name) == "afile" && utility.FromStringPtr(f.Link) == "https://new.example/x2" {
					found = true
				}
			}
			assert.True(t, found)
		},
		"SignedURLSuccess": func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection, user.Collection))

			tsk := task.Task{Id: "s1", BuildId: "b1", DisplayName: "disp", Execution: 0}
			require.NoError(t, tsk.Insert(t.Context()))
			entry := artifact.Entry{
				TaskId:          tsk.Id,
				TaskDisplayName: tsk.DisplayName,
				BuildId:         tsk.BuildId,
				Execution:       0,
				Files: []artifact.File{
					{
						Name:    "signed_file",
						Link:    "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/task_id/old.log?Token=abc&Expires=123",
						FileKey: "evergreen/task_id/old.log",
					},
				},
			}
			require.NoError(t, entry.Upsert(t.Context()))

			projCtx := serviceModel.Context{Task: &tsk}
			u := &user.DBUser{Id: "userSigned"}
			require.NoError(t, u.Insert(t.Context()))
			ctxWithUser := gimlet.AttachUser(ctx, u)
			ctxWithProj := context.WithValue(ctxWithUser, RequestContext, &projCtx)

			body := map[string]string{
				"artifact_name": "signed_file",
				"current_url":   "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/task_id/old.log?Token=abc&Expires=123",
				"new_url":       "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/task_id/new.log?Token=xyz&Expires=456",
			}
			data, _ := json.Marshal(body)
			h := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			req, _ := http.NewRequest(http.MethodPatch, "/tasks/s1/artifacts/url", bytes.NewReader(data))
			req = gimlet.SetURLVars(req, map[string]string{"task_id": "s1"})
			require.NoError(t, h.Parse(ctxWithProj, req))
			assert.True(t, h.isSignedURL)
			assert.Equal(t, "evergreen/task_id/old.log", h.currentFileKey)
			assert.Equal(t, "evergreen/task_id/new.log", h.newFileKey)

			resp := h.Run(ctxWithProj)
			assert.Equal(t, http.StatusOK, resp.Status())

			// Verify the artifact was updated in the database.
			updatedEntry, err := artifact.FindOne(ctx, artifact.ByTaskIdAndExecution(tsk.Id, 0))
			require.NoError(t, err)
			require.NotNil(t, updatedEntry)
			require.Len(t, updatedEntry.Files, 1)
			assert.Equal(t, "signed_file", updatedEntry.Files[0].Name)
			assert.Equal(t, "evergreen/task_id/new.log", updatedEntry.Files[0].FileKey)
		},
		"SignedURLValidationErrors": func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection, user.Collection))

			tsk := task.Task{Id: "s2", BuildId: "b1", DisplayName: "disp", Execution: 0}
			require.NoError(t, tsk.Insert(t.Context()))

			projCtx := serviceModel.Context{Task: &tsk}
			u := &user.DBUser{Id: "userSigned2"}
			require.NoError(t, u.Insert(t.Context()))
			ctxWithUser := gimlet.AttachUser(ctx, u)
			ctxWithProj := context.WithValue(ctxWithUser, RequestContext, &projCtx)

			// Test invalid current URL.
			body1 := map[string]string{
				"artifact_name": "signed_file",
				"current_url":   "https://example.com/notans3url?Token=abc",
				"new_url":       "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/task_id/new.log?Token=xyz&Expires=456",
			}
			data1, _ := json.Marshal(body1)
			h1 := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			req1, _ := http.NewRequest(http.MethodPatch, "/tasks/s2/artifacts/url", bytes.NewReader(data1))
			req1 = gimlet.SetURLVars(req1, map[string]string{"task_id": "s2"})
			err1 := h1.Parse(ctxWithProj, req1)
			require.Error(t, err1)
			assert.Contains(t, err1.Error(), "current_url must be a valid S3 URL")

			// Test invalid new URL.
			body2 := map[string]string{
				"artifact_name": "signed_file",
				"current_url":   "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/task_id/old.log?Token=abc&Expires=123",
				"new_url":       "https://example.com/notans3url?Token=xyz",
			}
			data2, _ := json.Marshal(body2)
			h2 := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			req2, _ := http.NewRequest(http.MethodPatch, "/tasks/s2/artifacts/url", bytes.NewReader(data2))
			req2 = gimlet.SetURLVars(req2, map[string]string{"task_id": "s2"})
			err2 := h2.Parse(ctxWithProj, req2)
			require.Error(t, err2)
			require.ErrorContains(t, err2, "new_url must be a valid S3 URL")

			// Test different buckets.
			body3 := map[string]string{
				"artifact_name": "signed_file",
				"current_url":   "https://bucket1.s3.us-east-1.amazonaws.com/path/old.log?Token=abc&Expires=123",
				"new_url":       "https://bucket2.s3.us-east-1.amazonaws.com/path/new.log?Token=xyz&Expires=456",
			}
			data3, _ := json.Marshal(body3)
			h3 := makeUpdateArtifactURLRoute().(*updateArtifactURLHandler)
			req3, _ := http.NewRequest(http.MethodPatch, "/tasks/s2/artifacts/url", bytes.NewReader(data3))
			req3 = gimlet.SetURLVars(req3, map[string]string{"task_id": "s2"})
			err3 := h3.Parse(ctxWithProj, req3)
			require.Error(t, err3)
			assert.Contains(t, err3.Error(), "current_url and new_url must be in the same S3 bucket")
		},
	} {
		t.Run(name, test)
	}
}

func TestParseS3URL(t *testing.T) {
	testCases := []struct {
		name           string
		url            string
		expectedBucket string
		expectedKey    string
	}{
		{
			name:           "VirtualHostedStyleNoRegion",
			url:            "https://mybucket.s3.amazonaws.com/path/to/file.log",
			expectedBucket: "mybucket",
			expectedKey:    "path/to/file.log",
		},
		{
			name:           "VirtualHostedStyleWithRegion",
			url:            "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/path/file.log",
			expectedBucket: "mciuploads",
			expectedKey:    "evergreen/path/file.log",
		},
		{
			name:           "VirtualHostedStyleWithPresignedURL",
			url:            "https://mciuploads.s3.us-east-1.amazonaws.com/evergreen/path/file.log?X-Amz-Algorithm=fake&X-Amz-Date=20251118T224351Z",
			expectedBucket: "mciuploads",
			expectedKey:    "evergreen/path/file.log",
		},
		{
			name:           "PathStyleNoRegion",
			url:            "https://s3.amazonaws.com/mybucket/path/to/file.log",
			expectedBucket: "mybucket",
			expectedKey:    "path/to/file.log",
		},
		{
			name:           "PathStyleWithRegion",
			url:            "https://s3.us-west-2.amazonaws.com/mybucket/path/to/file.log",
			expectedBucket: "mybucket",
			expectedKey:    "path/to/file.log",
		},
		{
			name:           "NonS3URL",
			url:            "https://example.com/path/to/file.log",
			expectedBucket: "",
			expectedKey:    "",
		},
		{
			name:           "InvalidURL",
			url:            "not a url",
			expectedBucket: "",
			expectedKey:    "",
		},
		{
			name:           "PathStyleBucketOnly",
			url:            "https://s3.amazonaws.com/mybucket",
			expectedBucket: "mybucket",
			expectedKey:    "",
		},
		{
			name:           "VirtualHostedStyleEmptyPath",
			url:            "https://mybucket.s3.amazonaws.com",
			expectedBucket: "mybucket",
			expectedKey:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bucket, key := parseS3URL(tc.url)
			assert.Equal(t, tc.expectedBucket, bucket)
			assert.Equal(t, tc.expectedKey, key)
		})
	}
}
