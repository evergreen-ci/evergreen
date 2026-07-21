package route

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectTestsHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	j := []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ := http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth := makeSelectTestsHandler(env)
	require.NoError(t, sth.Parse(ctx, req), "request should parse successfully")

	j = []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task"
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.NoError(t, sth.Parse(ctx, req), "request should parse successfully when tests is missing")

	j = []byte(`{
		"project_id": "",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.Error(t, sth.Parse(ctx, req), "request should fail to parse when project is missing")

	j = []byte(`{
		"project_id": "my-project",
		"requester": "",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.Error(t, sth.Parse(ctx, req), "request should fail to parse when requester is missing")

	j = []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.Error(t, sth.Parse(ctx, req))

	j = []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.Error(t, sth.Parse(ctx, req), "request should fail to parse when task ID is missing")

	j = []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.Error(t, sth.Parse(ctx, req), "request should fail to parse when task name is missing")
}

func TestMakeSelectTestsErrorResponse(t *testing.T) {
	t.Run("DeadlineExceededShouldReturnFailedDependency", func(t *testing.T) {
		resp := makeSelectTestsErrorResponse(errors.Wrap(context.DeadlineExceeded, "selecting tests"))
		assert.Equal(t, http.StatusFailedDependency, resp.Status())
	})

	t.Run("OtherErrorsShouldReturnInternalServerError", func(t *testing.T) {
		resp := makeSelectTestsErrorResponse(errors.New("selecting tests"))
		assert.Equal(t, http.StatusInternalServerError, resp.Status())
	})
}

// Regression guard: user-authenticated requests must reach the handler.
// Previously NewUserOrTaskAuthMiddleware short-circuited them on a missing
// {task_id} URL var that /select/tests doesn't have.
func TestSelectTestsRouteUserAuth(t *testing.T) {
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "test-user"})
	body := []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1"]
	}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/select/tests", bytes.NewBuffer(body))
	require.NoError(t, err)

	rw := httptest.NewRecorder()
	called := false
	NewUserOrTaskAuthOnlyMiddleware().ServeHTTP(rw, req, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	assert.True(t, called, "next handler should run for a user-authenticated request")
	assert.Equal(t, http.StatusOK, rw.Code, "middleware should not short-circuit user-authenticated requests")
}

// Unauthenticated requests should be rejected with 401.
func TestSelectTestsRouteUnauthenticated(t *testing.T) {
	body := []byte(`{
		"project_id": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1"]
	}`)
	req, err := http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(body))
	require.NoError(t, err)

	rw := httptest.NewRecorder()
	called := false
	NewUserOrTaskAuthOnlyMiddleware().ServeHTTP(rw, req, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	assert.False(t, called, "next handler should not run for unauthenticated requests")
	assert.Equal(t, http.StatusUnauthorized, rw.Code, "unauthenticated requests should be rejected with 401")
}

func TestSelectTestsHandlerAcceptsLegacyProjectKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	j := []byte(`{
		"project": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1"]
	}`)
	req, _ := http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth := makeSelectTestsHandler(env)
	require.NoError(t, sth.Parse(ctx, req), "request should parse with legacy 'project' key")
}
