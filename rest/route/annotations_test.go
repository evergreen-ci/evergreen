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
		{Id: "task-without-api-annotation", BuildId: "b1"},
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
			Id:             "1",
			TaskId:         "task-with-many-executions",
			TaskExecution:  0,
			APIAnnotation:  &annotations.Annotation{Note: &annotations.Note{Message: "note"}},
			UserAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}},
		},
		{
			Id:             "2",
			TaskId:         "task-with-many-executions",
			TaskExecution:  1,
			UserAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}}},
		{
			Id:            "3",
			TaskId:        "task-with-many-executions",
			TaskExecution: 2,
			APIAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "note"}}},
		{
			Id:             "4",
			TaskId:         "task-without-api-annotations",
			UserAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}}},
		{
			Id:            "5",
			TaskId:        "wrong-build",
			APIAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}}},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Insert())
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1) // only most recent execution
	assert.Equal(t, 2, apiAnnotations[0].TaskExecution)

	h.fetchAllExecutions = true
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 2) // all executions for which APIAnnotation is populated
	for _, a := range apiAnnotations {
		assert.Equal(t, "task-with-many-executions", model.FromStringPtr(a.TaskId))
		assert.Equal(t, "note", model.FromStringPtr(a.APIAnnotation.Note.Message))
		assert.NotEqual(t, 1, a.TaskExecution)
		assert.Nil(t, a.UserAnnotation)
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
		{Id: "task-without-api-annotation", Version: "v1"},
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
			Id:             "1",
			TaskId:         "task-with-many-executions",
			TaskExecution:  0,
			APIAnnotation:  &annotations.Annotation{Note: &annotations.Note{Message: "note"}},
			UserAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}},
		},
		{
			Id:             "2",
			TaskId:         "task-with-many-executions",
			TaskExecution:  1,
			UserAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}}},
		{
			Id:            "3",
			TaskId:        "task-with-many-executions",
			TaskExecution: 2,
			APIAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "note"}}},
		{
			Id:             "4",
			TaskId:         "task-without-api-annotations",
			UserAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}}},
		{
			Id:            "5",
			TaskId:        "wrong-version",
			APIAnnotation: &annotations.Annotation{Note: &annotations.Note{Message: "this note won't come up"}}},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Insert())
	}

	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	require.Len(t, apiAnnotations, 1) // only most recent execution
	assert.Equal(t, 2, apiAnnotations[0].TaskExecution)

	h.fetchAllExecutions = true
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	apiAnnotations = resp.Data().([]model.APITaskAnnotation)
	assert.Len(t, apiAnnotations, 2) // all executions for which APIAnnotation is populated
	for _, a := range apiAnnotations {
		assert.Equal(t, "task-with-many-executions", model.FromStringPtr(a.TaskId))
		assert.Equal(t, "note", model.FromStringPtr(a.APIAnnotation.Note.Message))
		assert.NotEqual(t, 1, a.TaskExecution)
		assert.Nil(t, a.UserAnnotation)
	}
}
