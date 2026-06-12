package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTaskQuarantineMockTSS(t *testing.T) {
	original := evergreen.GetEnvironment().Settings().TestSelection.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("null"))
	}))
	evergreen.GetEnvironment().Settings().TestSelection.URL = srv.URL
	t.Cleanup(func() {
		srv.Close()
		evergreen.GetEnvironment().Settings().TestSelection.URL = original
	})
}

func ctxWithTask(t *task.Task) context.Context {
	ctx := context.WithValue(context.Background(), RequestContext, &serviceModel.Context{Task: t})
	return gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})
}

func TestTaskQuarantineExecutionTaskSucceeds(t *testing.T) {
	setupTaskQuarantineMockTSS(t)

	tsk := &task.Task{Id: "execution_task", Project: "p1", BuildVariant: "ubuntu", DisplayName: "my_task"}
	ctx := ctxWithTask(tsk)

	h := makeTaskQuarantineHandler()
	require.NoError(t, h.Parse(ctx, nil))
	res := h.Run(ctx)

	assert.Equal(t, http.StatusOK, res.Status())
	apiTask, ok := res.Data().(*model.APITask)
	require.True(t, ok)
	assert.Equal(t, utility.ToStringPtr("execution_task"), apiTask.Id)
}

func TestTaskQuarantineDisplayTaskIsRejected(t *testing.T) {
	setupTaskQuarantineMockTSS(t)

	tsk := &task.Task{Id: "display_task", Project: "p1", BuildVariant: "ubuntu", DisplayName: "display", DisplayOnly: true}
	ctx := ctxWithTask(tsk)

	h := makeTaskQuarantineHandler()
	err := h.Parse(ctx, nil)
	require.Error(t, err)
	resp, ok := err.(gimlet.ErrorResponse)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Message, "cannot modify quarantine state on display task 'display_task'")
}

func TestTaskQuarantineParseRequiresTaskInContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), RequestContext, &serviceModel.Context{Task: nil})

	h := makeTaskQuarantineHandler()
	err := h.Parse(ctx, nil)
	require.Error(t, err)
	resp, ok := err.(gimlet.ErrorResponse)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTaskQuarantineTSSFailurePropagates(t *testing.T) {
	original := evergreen.GetEnvironment().Settings().TestSelection.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	evergreen.GetEnvironment().Settings().TestSelection.URL = srv.URL
	t.Cleanup(func() {
		srv.Close()
		evergreen.GetEnvironment().Settings().TestSelection.URL = original
	})

	tsk := &task.Task{Id: "execution_task", Project: "p1", BuildVariant: "ubuntu", DisplayName: "my_task"}
	ctx := ctxWithTask(tsk)

	h := makeTaskQuarantineHandler()
	require.NoError(t, h.Parse(ctx, nil))
	res := h.Run(ctx)

	assert.Equal(t, http.StatusInternalServerError, res.Status())
	assert.NotNil(t, res.Data())
}
