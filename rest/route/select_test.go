package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectTestsHandler(t *testing.T) {
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
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ := http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth := makeSelectTestsHandler(env)
	require.NoError(t, sth.Parse(ctx, req), "request should parse successfully")

	j = []byte(`{
		"project": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task"
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler(env)
	require.NoError(t, sth.Parse(ctx, req), "request should parse successfully when tests is missing")

	j = []byte(`{
		"project": "",
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
		"project": "my-project",
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
		"project": "my-project",
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
		"project": "my-project",
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
		"project": "my-project",
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

func TestSelectTestsHandlerProjectField(t *testing.T) {
	for _, tc := range []struct {
		name            string
		body            string
		expectedProject string
	}{
		{
			name:            "CanonicalProjectIDIsAccepted",
			body:            `{"project_id": "my-project", "requester": "patch", "build_variant": "variant", "task_id": "my-task-1234", "task_name": "my-task"}`,
			expectedProject: "my-project",
		},
		{
			name:            "LegacyProjectIsAccepted",
			body:            `{"project": "my-project", "requester": "patch", "build_variant": "variant", "task_id": "my-task-1234", "task_name": "my-task"}`,
			expectedProject: "my-project",
		},
		{
			name:            "ProjectIDTakesPrecedenceOverLegacyProject",
			body:            `{"project_id": "canonical", "project": "legacy", "requester": "patch", "build_variant": "variant", "task_id": "my-task-1234", "task_name": "my-task"}`,
			expectedProject: "canonical",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBufferString(tc.body))
			require.NoError(t, err)
			h, ok := makeSelectTestsHandler(&mock.Environment{}).(*selectTestsHandler)
			require.True(t, ok)
			require.NoError(t, h.Parse(t.Context(), req))
			assert.Equal(t, tc.expectedProject, h.selectTests.Project)
		})
	}
}
