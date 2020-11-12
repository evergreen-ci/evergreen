package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationsByBuildHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationsByBuildHandler{}
	r, err := http.NewRequest("GET", "/builds/b1/annotations", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"build_id": "b1"})

	ctx := context.TODO()
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "b1", h.buildId)
	assert.False(t, h.fetchAllExecutions)

	r, err = http.NewRequest("GET", "/builds/b2/annotations?fetch_all_executions=true", nil)
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
		assert.NoError(t, each.Insert())
	}
	h := &annotationsByBuildHandler{
		sc:      &data.DBConnector{},
		buildId: "b1",
	}
	ctx := context.TODO()
	// no annotations doesn't error
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations := resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 0)

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
		assert.NoError(t, a.Insert())
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 2) // skip the previous execution of task-with-many-executions
	for _, a := range apiAnnotations {
		assert.NotEqual(t, 1, a.TaskExecution)
	}

	h.fetchAllExecutions = true
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 3)
	for _, a := range apiAnnotations {
		assert.NotEqual(t, "wrong-build", model.FromStringPtr(a.TaskId))
		assert.Equal(t, "note", model.FromStringPtr(a.Note.Message))
	}
}

func TestAnnotationsByVersionHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationsByVersionHandler{}
	r, err := http.NewRequest("GET", "/versions/v1/annotations", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"version_id": "v1"})

	ctx := context.TODO()
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "v1", h.versionId)
	assert.False(t, h.fetchAllExecutions)

	r, err = http.NewRequest("GET", "/versions/v2/annotations?fetch_all_executions=true", nil)
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
		assert.NoError(t, each.Insert())
	}
	h := &annotationsByVersionHandler{
		sc:        &data.DBConnector{},
		versionId: "v1",
	}
	ctx := context.TODO()
	// no annotations doesn't error
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations := resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 0)

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
		assert.NoError(t, a.Insert())
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 2) // skip the previous execution of task-with-many-executions
	for _, a := range apiAnnotations {
		assert.NotEqual(t, 1, a.TaskExecution)
	}

	h.fetchAllExecutions = true
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 3)
	for _, a := range apiAnnotations {
		assert.NotEqual(t, "wrong-build", model.FromStringPtr(a.TaskId))
		assert.Equal(t, "note", model.FromStringPtr(a.Note.Message))
	}
}

func TestAnnotationByTaskHandlerParse(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection))
	h := &annotationByTaskHandler{}
	r, err := http.NewRequest("GET", "/task/t1/annotations", nil)
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

	r, err = http.NewRequest("GET", "/task/t2/annotations?execution=1", nil)
	assert.NoError(t, err)
	vars = map[string]string{
		"task_id": "t2",
	}
	r = gimlet.SetURLVars(r, vars)
	assert.NoError(t, h.Parse(ctx, r))
	assert.Equal(t, "t2", h.taskId)
	assert.Equal(t, 1, h.execution)

	r, err = http.NewRequest("GET", "/task/t2/annotations?fetch_all_executions=true", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, vars)
	assert.NoError(t, h.Parse(ctx, r))
	assert.True(t, h.fetchAllExecutions)

	// do not allow fetching all executions and fetching a specific execution at the same time
	r, err = http.NewRequest("GET", "/task/t2/annotations?fetch_all_executions=true&execution=1", nil)
	assert.NoError(t, err)
	vars = map[string]string{
		"task_id": "t2",
	}
	r = gimlet.SetURLVars(r, vars)
	err = h.Parse(ctx, r)

	assert.Contains(t, err.Error(), "fetchAllExecutions=true cannot be combined with execution={task_execution}")
}

func TestAnnotationByTaskHandlerRun(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.Collection, task.Collection, task.OldCollection))
	tasks := []task.Task{
		{Id: "task-1", Execution: 0},
		{Id: "task-2", Execution: 0},
	}
	for _, each := range tasks {
		assert.NoError(t, each.Insert())
	}
	// add task-1, execution 1
	assert.NoError(t, task.ResetTasks([]string{"task-1"}))

	h := &annotationByTaskHandler{
		sc:                 &data.DBConnector{},
		taskId:             "task-1",
		execution:          -1, //unspecified
		fetchAllExecutions: false,
	}
	ctx := context.TODO()
	// no annotations doesn't error
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status()) //works
	apiAnnotations := resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 0)

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
		},
		{
			Id:            "4",
			TaskId:        "task-2",
			TaskExecution: 0,
			Note:          &annotations.Note{Message: "task-2-note_0"},
		},
	}

	for _, a := range annotations {
		assert.NoError(t, a.Insert())
	}

	// get the latest execution : 1
	// works
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1)
	require.NotNil(t, apiAnnotations)
	assert.Equal(t, 1, apiAnnotations[0].TaskExecution)
	assert.Equal(t, "task-1", model.FromStringPtr(apiAnnotations[0].TaskId))
	assert.Equal(t, "task-1-note_1", model.FromStringPtr(apiAnnotations[0].Note.Message))

	// get the latest execution : 0
	h.taskId = "task-2"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1)
	require.NotNil(t, apiAnnotations)
	assert.Equal(t, 0, apiAnnotations[0].TaskExecution)
	assert.Equal(t, "task-2", model.FromStringPtr(apiAnnotations[0].TaskId))
	assert.Equal(t, "task-2-note_0", model.FromStringPtr(apiAnnotations[0].Note.Message))

	// get a specific execution :0
	h.execution = 0
	h.taskId = "task-1"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1)
	require.NotNil(t, apiAnnotations)
	assert.Equal(t, 0, apiAnnotations[0].TaskExecution)
	assert.Equal(t, "task-1", model.FromStringPtr(apiAnnotations[0].TaskId))
	assert.Equal(t, "task-1-note_0", model.FromStringPtr(apiAnnotations[0].Note.Message))

	// fetch all executions
	h.execution = -1
	h.fetchAllExecutions = true
	h.taskId = "task-1"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 2)
	require.NotNil(t, apiAnnotations)
	for _, a := range apiAnnotations {
		assert.NotEqual(t, "task-1", a.TaskId)
	}
}
