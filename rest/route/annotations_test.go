package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationsByBuildHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationsByBuildHandler{}
	r, err := http.NewRequest(http.MethodGet, "/builds/b1/annotations", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"build_id": "b1"})

	ctx := context.TODO()
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "b1", h.buildId)
	assert.False(t, h.fetchAllExecutions)

	r, err = http.NewRequest(http.MethodGet, "/builds/b2/annotations?fetch_all_executions=true", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"build_id": "b2"})
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "b2", h.buildId)
	assert.True(t, h.fetchAllExecutions)
}

func TestAnnotationsByBuildHandlerRun(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection, task.Collection))
	tasks := []task.Task{
		{Id: "task-with-many-executions", BuildId: "b1"},
		{Id: "other-task", BuildId: "b1"},
		{Id: "wrong-build", BuildId: "b2"},
	}
	for _, each := range tasks {
		assert.NoError(t, each.Insert(t.Context()))
	}
	h := &annotationsByBuildHandler{
		buildId: "b1",
	}
	ctx := context.TODO()
	// no annotations doesn't error
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations := resp.Data().([]restModel.APITaskAnnotation)
	assert.Empty(t, apiAnnotations)

	annotations := []annotations.TaskAnnotation{
		{
			Id:            "1",
			TaskId:        "task-with-many-executions",
			TaskExecution: 1,
			Note:          &annotations.Note{Message: "note"},
		},
		{
			Id:            "2",
			TaskId:        "task-with-many-executions",
			TaskExecution: 2,
			Note:          &annotations.Note{Message: "note"},
		},
		{
			Id:            "3",
			TaskId:        "other-task",
			TaskExecution: 0,
			Note:          &annotations.Note{Message: "note"},
		},
		{
			Id:     "4",
			TaskId: "wrong-build",
			Note:   &annotations.Note{Message: "this note won't come up"},
		},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Upsert(t.Context()))
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	require.Len(t, apiAnnotations, 2) // skip the previous execution of task-with-many-executions
	for _, a := range apiAnnotations {
		assert.NotEqual(t, 1, a.TaskExecution)
	}

	h.fetchAllExecutions = true
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 3)
	for _, a := range apiAnnotations {
		assert.NotEqual(t, "wrong-build", utility.FromStringPtr(a.TaskId))
		assert.Equal(t, "note", utility.FromStringPtr(a.Note.Message))
	}
}

func TestAnnotationsByVersionHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationsByVersionHandler{}
	r, err := http.NewRequest(http.MethodGet, "/versions/v1/annotations", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"version_id": "v1"})

	ctx := context.TODO()
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "v1", h.versionId)
	assert.False(t, h.fetchAllExecutions)

	r, err = http.NewRequest(http.MethodGet, "/versions/v2/annotations?fetch_all_executions=true", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"version_id": "v2"})
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "v2", h.versionId)
	assert.True(t, h.fetchAllExecutions)
}

func TestAnnotationsByVersionHandlerRun(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection, task.Collection))
	tasks := []task.Task{
		{Id: "task-with-many-executions", Version: "v1"},
		{Id: "other-task", Version: "v1"},
		{Id: "wrong-build", Version: "v2"},
	}
	for _, each := range tasks {
		assert.NoError(t, each.Insert(t.Context()))
	}
	h := &annotationsByVersionHandler{
		versionId: "v1",
	}
	ctx := context.TODO()
	// no annotations doesn't error
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations := resp.Data().([]restModel.APITaskAnnotation)
	assert.Empty(t, apiAnnotations)

	annotations := []annotations.TaskAnnotation{
		{
			Id:            "1",
			TaskId:        "task-with-many-executions",
			TaskExecution: 1,
			Note:          &annotations.Note{Message: "note"},
		},
		{
			Id:            "2",
			TaskId:        "task-with-many-executions",
			TaskExecution: 2,
			Note:          &annotations.Note{Message: "note"},
		},
		{
			Id:            "3",
			TaskId:        "other-task",
			TaskExecution: 0,
			Note:          &annotations.Note{Message: "note"},
		},
		{
			Id:     "4",
			TaskId: "wrong-build",
			Note:   &annotations.Note{Message: "this note won't come up"},
		},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Upsert(t.Context()))
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	require.Len(t, apiAnnotations, 2) // skip the previous execution of task-with-many-executions
	for _, a := range apiAnnotations {
		assert.NotEqual(t, 1, a.TaskExecution)
	}

	h.fetchAllExecutions = true
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 3)
	for _, a := range apiAnnotations {
		assert.NotEqual(t, "wrong-build", utility.FromStringPtr(a.TaskId))
		assert.Equal(t, "note", utility.FromStringPtr(a.Note.Message))
	}
}

func TestAnnotationByTaskGetHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationByTaskGetHandler{}
	r, err := http.NewRequest(http.MethodGet, "/task/t1/annotations", nil)
	assert.NoError(t, err)
	vars := map[string]string{
		"task_id": "t1",
	}
	r = gimlet.SetURLVars(r, vars)
	ctx := context.TODO()
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "t1", h.taskId)
	// the default should be execution:-1, fetch_all_executions:false
	assert.Equal(t, -1, h.execution)
	assert.False(t, h.fetchAllExecutions)

	r, err = http.NewRequest(http.MethodGet, "/task/t2/annotations?execution=1", nil)
	assert.NoError(t, err)
	vars = map[string]string{
		"task_id": "t2",
	}
	r = gimlet.SetURLVars(r, vars)
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "t2", h.taskId)
	assert.Equal(t, 1, h.execution)

	r, err = http.NewRequest(http.MethodGet, "/task/t2/annotations?fetch_all_executions=true", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, vars)
	assert.NoError(t, h.Parse(ctx, r))
	assert.True(t, h.fetchAllExecutions)

	// do not allow fetching all executions and fetching a specific execution at the same time
	r, err = http.NewRequest(http.MethodGet, "/task/t2/annotations?fetch_all_executions=true&execution=1", nil)
	assert.NoError(t, err)
	vars = map[string]string{
		"task_id": "t2",
	}
	r = gimlet.SetURLVars(r, vars)
	err = h.Parse(ctx, r)

	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "cannot both fetch all executions and request a specific execution")
}

func TestAnnotationByTaskGetHandlerRun(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationByTaskGetHandler{
		taskId:             "task-1",
		execution:          -1, //unspecified
		fetchAllExecutions: false,
	}
	ctx := context.TODO()
	// no annotations doesn't error
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations := resp.Data().([]restModel.APITaskAnnotation)
	assert.Empty(t, apiAnnotations)

	annotations := []annotations.TaskAnnotation{
		{
			Id:            "1",
			TaskId:        "task-1",
			TaskExecution: 0,
			Note:          &annotations.Note{Message: "task-1-note_0"},
		},
		{
			Id:            "2",
			TaskId:        "task-1",
			TaskExecution: 1,
			Note:          &annotations.Note{Message: "task-1-note_1"},
			Issues: []annotations.IssueLink{
				{ConfidenceScore: 12.34},
			},
		},
		{
			Id:            "4",
			TaskId:        "task-2",
			TaskExecution: 0,
			Note:          &annotations.Note{Message: "task-2-note_0"},
		},
	}

	for _, a := range annotations {
		assert.NoError(t, a.Upsert(t.Context()))
	}

	// get the latest execution : 1
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1)
	require.NotNil(t, apiAnnotations)
	assert.Equal(t, 1, *apiAnnotations[0].TaskExecution)
	assert.Equal(t, "task-1", utility.FromStringPtr(apiAnnotations[0].TaskId))
	assert.Equal(t, "task-1-note_1", utility.FromStringPtr(apiAnnotations[0].Note.Message))
	require.Len(t, apiAnnotations[0].Issues, 1)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(12.34), utility.FromFloat64Ptr(apiAnnotations[0].Issues[0].ConfidenceScore))

	// get the latest execution : 0
	h.taskId = "task-2"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1)
	require.NotNil(t, apiAnnotations)
	assert.Equal(t, 0, *apiAnnotations[0].TaskExecution)
	assert.Equal(t, "task-2", utility.FromStringPtr(apiAnnotations[0].TaskId))
	assert.Equal(t, "task-2-note_0", utility.FromStringPtr(apiAnnotations[0].Note.Message))

	// get a specific execution :0
	h.execution = 0
	h.taskId = "task-1"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1)
	require.NotNil(t, apiAnnotations)
	assert.Equal(t, 0, *apiAnnotations[0].TaskExecution)
	assert.Equal(t, "task-1", utility.FromStringPtr(apiAnnotations[0].TaskId))
	assert.Equal(t, "task-1-note_0", utility.FromStringPtr(apiAnnotations[0].Note.Message))

	// fetch all executions
	h.execution = -1
	h.fetchAllExecutions = true
	h.taskId = "task-1"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]restModel.APITaskAnnotation)
	require.Len(t, apiAnnotations, 2)
	require.NotNil(t, apiAnnotations)
	for _, a := range apiAnnotations {
		assert.NotEqual(t, "task-1", a.TaskId)
	}
}

func TestAnnotationByTaskPutHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection, task.Collection, task.OldCollection))
	tasks := []task.Task{
		{Id: "TaskFailedId", Execution: 1, Status: evergreen.TaskFailed},
		{Id: "TaskSystemUnresponseId", Execution: 1, Status: evergreen.TaskSystemUnresponse},
		{Id: "TaskSystemFailedId", Execution: 0, Status: evergreen.TaskSystemFailed},
		{Id: "TaskTimedOutId", Execution: 0, Status: evergreen.TaskTimedOut},
		{Id: "TaskSetupFailedId", Execution: 0, Status: evergreen.TaskSetupFailed},
		{Id: "TaskSucceededId", Execution: 0, Status: evergreen.TaskSucceeded},
		{Id: "TaskWillRunId", Execution: 0, Status: evergreen.TaskWillRun},
		{Id: "TaskDispatchedId", Execution: 0, Status: evergreen.TaskDispatched},
	}

	old_tasks := []task.Task{
		{Id: "TaskFailedId_0", Execution: 0, Status: evergreen.TaskFailed},
		{Id: "t2_0", Execution: 0, Status: evergreen.TaskFailed},
	}

	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "test_annotation_user"})

	for _, each := range tasks {
		assert.NoError(t, each.Insert(t.Context()))
	}

	for _, each := range old_tasks {
		assert.NoError(t, each.Insert(t.Context()))
		assert.NoError(t, each.Archive(ctx))
	}

	h := &annotationByTaskPutHandler{}

	execution0 := 0
	execution1 := 1
	a := &restModel.APITaskAnnotation{
		Id:     utility.ToStringPtr("1"),
		TaskId: utility.ToStringPtr("TaskFailedId"),
		Note:   &restModel.APINote{Message: utility.ToStringPtr("task-1-note_0")},
	}

	jsonBody, err := json.Marshal(a)
	require.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=1", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	assert.NoError(t, h.Parse(ctx, r))

	assert.Equal(t, "TaskFailedId", h.taskId)
	// unspecified execution defaults to latest
	assert.Equal(t, &execution1, h.annotation.TaskExecution)
	assert.Equal(t, "task-1-note_0", utility.FromStringPtr(h.annotation.Note.Message))
	assert.Equal(t, "test_annotation_user", h.user.(*user.DBUser).Id)

	// test with an annotation with invalid URL in Issues
	h = &annotationByTaskPutHandler{}
	a.Issues = []restModel.APIIssueLink{
		{
			URL:             utility.ToStringPtr("issuelink.com"),
			ConfidenceScore: utility.ToFloat64Ptr(-12.000000),
		},
		{
			URL:             utility.ToStringPtr("https://issuelink.com/ticket"),
			ConfidenceScore: utility.ToFloat64Ptr(112.000000),
		},
	}
	a.SuspectedIssues = []restModel.APIIssueLink{
		{
			URL: utility.ToStringPtr("https://issuelinkcom"),
		},
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=1", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})

	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "parsing request URI 'issuelink.com'")
	assert.Contains(t, err.Error(), "URL 'https://issuelinkcom' must have a domain and extension")
	assert.Contains(t, err.Error(), "confidence score '-12.000000' must be between 0 and 100")
	assert.Contains(t, err.Error(), "confidence score '112.000000' must be between 0 and 100")

	//test with a task that doesn't exist
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("non-existent"),
		TaskExecution: &execution1,
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=1", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "non-existent"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "task 'non-existent' not found")

	//test with a request that mismatches task execution
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskFailedId"),
		TaskExecution: &execution1,
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=2", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "task execution number from query parameter (2) must equal the task execution number specified in the annotation (1)")

	//test with a request that omits task execution
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:     utility.ToStringPtr("1"),
		TaskId: utility.ToStringPtr("TaskFailedId"),
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "task execution must be specified in the request query parameter or the request body's annotation")

	//test with request that only has execution in the request body
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskFailedId"),
		TaskExecution: &execution1,
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)
	r, err = http.NewRequest(http.MethodPut, "/task/TaskSystemFailedId/annotations", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	assert.NoError(t, err)
	assert.Equal(t, &execution1, h.annotation.TaskExecution)

	//test with request that only has execution in the request url
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:     utility.ToStringPtr("1"),
		TaskId: utility.ToStringPtr("TaskFailedId"),
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)
	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=1", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	assert.NoError(t, err)
	assert.Equal(t, &execution1, h.annotation.TaskExecution)

	//test with a task that has an invalid task execution
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskFailedId"),
		TaskExecution: &execution1,
	}
	jsonBody, err = json.Marshal(a)
	assert.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=abc", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "converting execution to integer value")

	//test with empty taskId
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{}
	jsonBody, err = json.Marshal(a)
	assert.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)
	r, err = http.NewRequest(http.MethodPut, "/task/TaskFailedId/annotations?execution=1", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskFailedId"})
	assert.NoError(t, err)
	require.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "TaskFailedId", h.taskId)

	//test with id not equal to annotation id
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskSystemUnresponseId"),
		TaskExecution: &execution0,
	}
	jsonBody, err = json.Marshal(a)
	assert.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskSystemUnresponseId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskSystemFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "task ID parameter 'TaskSystemFailedId' must equal the task ID specified in the annotation 'TaskSystemUnresponseId'")

	//test with fail statuses
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)
	r, err = http.NewRequest(http.MethodPut, "/task/TaskSystemFailedId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskSystemFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	assert.NoError(t, err)
	assert.Equal(t, "TaskSystemFailedId", h.taskId)

	a = &restModel.APITaskAnnotation{}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)
	r, err = http.NewRequest(http.MethodPut, "/task/TaskTimedOutId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskTimedOutId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	assert.NoError(t, err)
	assert.Equal(t, "TaskTimedOutId", h.taskId)

	a = &restModel.APITaskAnnotation{}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)
	r, err = http.NewRequest(http.MethodPut, "/task/TaskSetupFailedId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskSetupFailedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	assert.NoError(t, err)
	assert.Equal(t, "TaskSetupFailedId", h.taskId)

	//test with task without fail status
	h = &annotationByTaskPutHandler{}
	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskSucceededId"),
		TaskExecution: &execution0,
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskSucceededId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskSucceededId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "cannot create annotation when task status is")

	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskWillRunId"),
		TaskExecution: &execution0,
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskWillRunId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskWillRunId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "cannot create annotation when task status is")

	a = &restModel.APITaskAnnotation{
		Id:            utility.ToStringPtr("1"),
		TaskId:        utility.ToStringPtr("TaskDispatchedId"),
		TaskExecution: &execution0,
	}
	jsonBody, err = json.Marshal(a)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/TaskDispatchedId/annotations?execution=0", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "TaskDispatchedId"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "cannot create annotation when task status is")

}

func TestAnnotationByTaskPutHandlerRun(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection, task.Collection))
	t1 := task.Task{Id: "t1"}
	require.NoError(t, t1.Insert(t.Context()))
	execution0 := 0
	execution1 := 1
	a := restModel.APITaskAnnotation{
		TaskId:        utility.ToStringPtr("t1"),
		TaskExecution: &execution0,
		Note:          &restModel.APINote{Message: utility.ToStringPtr("task-1-note_0")},
		Issues: []restModel.APIIssueLink{
			{
				URL:             utility.ToStringPtr("some_url_0"),
				IssueKey:        utility.ToStringPtr("some key 0"),
				ConfidenceScore: utility.ToFloat64Ptr(12.34),
			},
			{
				URL:             utility.ToStringPtr("some_url_1"),
				IssueKey:        utility.ToStringPtr("some key 1"),
				ConfidenceScore: utility.ToFloat64Ptr(56.78),
			},
		},
	}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "test_annotation_user"})

	//test insert
	h := &annotationByTaskPutHandler{
		taskId:     "t1",
		annotation: &a,
		user:       &user.DBUser{Id: "test_annotation_user"},
	}
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	annotation, err := annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	require.NoError(t, err)
	assert.NotEqual(t, "", annotation.Id)
	assert.Equal(t, "task-1-note_0", annotation.Note.Message)
	assert.Equal(t, "test_annotation_user", annotation.Note.Source.Author)
	assert.Equal(t, "api", annotation.Note.Source.Requester)
	assert.Equal(t, "api", annotation.Issues[0].Source.Requester)
	assert.Len(t, annotation.Issues, 2)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(12.34), annotation.Issues[0].ConfidenceScore)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(56.78), annotation.Issues[1].ConfidenceScore)

	//test update
	h.annotation = &restModel.APITaskAnnotation{
		TaskId:        utility.ToStringPtr("t1"),
		TaskExecution: &execution0,
		Note:          &restModel.APINote{Message: utility.ToStringPtr("task-1-note_0_updated")},
		Issues: []restModel.APIIssueLink{
			{
				URL:             utility.ToStringPtr("some_url_0"),
				IssueKey:        utility.ToStringPtr("some key 0"),
				ConfidenceScore: utility.ToFloat64Ptr(87.65),
			},
			{
				URL:             utility.ToStringPtr("some_url_1"),
				IssueKey:        utility.ToStringPtr("some key 1"),
				ConfidenceScore: utility.ToFloat64Ptr(43.21),
			},
		},
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	annotation, err = annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	require.NoError(t, err)
	assert.NotEqual(t, "", annotation.Id)
	assert.Equal(t, "task-1-note_0_updated", annotation.Note.Message)
	// suspected issues and issues don't get updated when not defined
	require.Nil(t, annotation.SuspectedIssues)
	assert.Equal(t, "some key 0", annotation.Issues[0].IssueKey)
	assert.Len(t, annotation.Issues, 2)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(87.65), annotation.Issues[0].ConfidenceScore)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(43.21), annotation.Issues[1].ConfidenceScore)

	//test that it can update old executions
	h.annotation = &restModel.APITaskAnnotation{
		TaskId:        utility.ToStringPtr("t1"),
		TaskExecution: &execution1,
		Note:          &restModel.APINote{Message: utility.ToStringPtr("task-1-note_1_updated")},
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	annotation, err = annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 1)
	require.NoError(t, err)
	assert.Equal(t, "task-1-note_1_updated", annotation.Note.Message)
}

// test created tickets route
func TestCreatedTicketByTaskPutHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection, task.Collection, task.OldCollection, model.ProjectRefCollection))
	testProject := "testProject"
	p := model.ProjectRef{
		Identifier: testProject,
	}
	assert.NoError(t, p.Insert(t.Context()))
	tasks := []task.Task{
		{Id: "t1", Execution: 1, Project: testProject},
		{Id: "t2", Execution: 1, Project: testProject},
	}
	for _, each := range tasks {
		assert.NoError(t, each.Insert(t.Context()))
	}
	h := &createdTicketByTaskPutHandler{}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "test_annotation_user"})

	url := utility.ToStringPtr("https://issuelink.com")
	ticket := &restModel.APIIssueLink{
		URL:      url,
		IssueKey: utility.ToStringPtr("some key 0"),
	}

	jsonBody, err := json.Marshal(ticket)
	require.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)
	p.TaskAnnotationSettings = evergreen.AnnotationsSettings{
		FileTicketWebhook: evergreen.WebHook{
			Endpoint: "random",
		},
	}
	assert.NoError(t, p.Replace(t.Context()))
	r, err := http.NewRequest(http.MethodPut, "/task/t1/created_ticket?execution=1", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "t1"})
	assert.NoError(t, err)
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "t1", h.taskId)
	assert.Equal(t, 1, h.execution)
	assert.Equal(t, "test_annotation_user", h.user.(*user.DBUser).Id)
	assert.Equal(t, url, h.ticket.URL)

	// test with an invalid URL
	h = &createdTicketByTaskPutHandler{}
	ticket.URL = utility.ToStringPtr("issuelink.com")
	jsonBody, err = json.Marshal(ticket)
	require.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodPut, "/task/t1/annotations?execution=1", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "t1"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "parsing request URI 'issuelink.com'")

	// test with a task that doesn't exist
	h = &createdTicketByTaskPutHandler{}

	r, err = http.NewRequest(http.MethodPut, "/task/t1/annotations?execution=1", buffer)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "non-existent"})
	assert.NoError(t, err)
	err = h.Parse(ctx, r)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "task 'non-existent' not found")
}

func TestCreatedTicketByTaskPutHandlerRun(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))

	ticket := &restModel.APIIssueLink{
		URL:      utility.ToStringPtr("https://issuelink1.com"),
		IssueKey: utility.ToStringPtr("Issue_key_1"),
	}

	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "test_annotation_user"})

	//test when there is no annotation for the task
	h := &createdTicketByTaskPutHandler{
		taskId:    "t1",
		execution: 0,
		ticket:    ticket,
		user:      &user.DBUser{Id: "test_annotation_user"},
	}
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	annotation, err := annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	require.NoError(t, err)
	assert.NotEqual(t, "", annotation.Id)
	assert.Equal(t, "https://issuelink1.com", annotation.CreatedIssues[0].URL)
	assert.Equal(t, "Issue_key_1", annotation.CreatedIssues[0].IssueKey)

	// add a ticket to the existing annotation
	h.ticket = &restModel.APIIssueLink{
		URL:      utility.ToStringPtr("https://issuelink2.com"),
		IssueKey: utility.ToStringPtr("Issue_key_2"),
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	annotation, err = annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	require.NoError(t, err)
	assert.NotEqual(t, "", annotation.Id)
	assert.Equal(t, "https://issuelink1.com", annotation.CreatedIssues[0].URL)
	assert.Equal(t, "Issue_key_1", annotation.CreatedIssues[0].IssueKey)
	assert.Equal(t, "https://issuelink2.com", annotation.CreatedIssues[1].URL)
	assert.Equal(t, "Issue_key_2", annotation.CreatedIssues[1].IssueKey)
}
