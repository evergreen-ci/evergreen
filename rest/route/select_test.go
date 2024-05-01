package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, sth.Parse(ctx, req))

	result := sth.Run(ctx)
	selectRequest := result.Data().(SelectTestsRequest)
	assert.Equal(t, "my-project", selectRequest.Project)
	assert.Equal(t, "patch", selectRequest.Requester)
	assert.Equal(t, "variant", selectRequest.BuildVariant)
	assert.Equal(t, "my-task-1234", selectRequest.TaskID)
	assert.Equal(t, "my-task", selectRequest.TaskName)
	assert.Equal(t, []string{"test1", "test2", "test3"}, selectRequest.Tests)

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
	assert.Error(t, sth.Parse(ctx, req))

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
	assert.Error(t, sth.Parse(ctx, req))

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
	assert.Error(t, sth.Parse(ctx, req))

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
	assert.Error(t, sth.Parse(ctx, req))

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
	assert.Error(t, sth.Parse(ctx, req))

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
	assert.Error(t, sth.Parse(ctx, req))
}
