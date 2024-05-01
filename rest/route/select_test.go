package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectTestsHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := []byte(`{
		"project": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ := http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth := makeSelectTestsHandler()
	require.NoError(t, sth.Parse(ctx, req), "request should parse successfully")

	result := sth.Run(ctx)
	selectRequest := result.Data().(SelectTestsRequest)
	require.Equal(t, "my-project", selectRequest.Project)
	require.Equal(t, "patch", selectRequest.Requester)
	require.Equal(t, "variant", selectRequest.BuildVariant)
	require.Equal(t, "my-task-1234", selectRequest.TaskID)
	require.Equal(t, "my-task", selectRequest.TaskName)
	require.Equal(t, []string{"test1", "test2", "test3"}, selectRequest.Tests)

	j = []byte(`{
		"project": "",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "my-task",
		"tests": ["test1", "test2", "test3"]
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler()
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
	sth = makeSelectTestsHandler()
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
	sth = makeSelectTestsHandler()
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
	sth = makeSelectTestsHandler()
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
	sth = makeSelectTestsHandler()
	require.Error(t, sth.Parse(ctx, req), "request should fail to parse when task name is missing")

	j = []byte(`{
		"project": "my-project",
		"requester": "patch",
		"build_variant": "variant",
		"task_id": "my-task-1234",
		"task_name": "",
		"tests": []
	}`)
	req, _ = http.NewRequest(http.MethodPost, "/select/tests", bytes.NewBuffer(j))
	sth = makeSelectTestsHandler()
	require.Error(t, sth.Parse(ctx, req), "request should fail to parse when tests are empty")
}
