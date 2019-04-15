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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Thing struct {
	Thing string `json:"thing"`
}

func TestValidateJSON(t *testing.T) {
	assert := assert.New(t)
	jsonBytes := []byte(`
[
{
  "thing": "one"
},
{
  "thing": "two"
}
]
`)
	buffer := bytes.NewBuffer(jsonBytes)
	request, err := http.NewRequest("", "", buffer)
	assert.NoError(err)
	files, err := parseJson(request)
	assert.NoError(err)
	var thing Thing
	for i, f := range files {
		assert.NoError(json.Unmarshal(f, &thing))
		switch i {
		case 0:
			assert.Equal("one", thing.Thing)
		case 1:
			assert.Equal("two", thing.Thing)
		}
	}
}

func TestGenerateExecute(t *testing.T) {
	h := &generateHandler{sc: &data.MockConnector{}}
	r := h.Run(context.Background())
	assert.Equal(t, r.Data(), struct{}{})
	assert.Equal(t, r.Status(), http.StatusOK)
}

func TestGeneratePollParse(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, host.Collection))

	sc := &data.MockConnector{}
	ctx := context.Background()

	r, err := http.NewRequest("GET", "/task/1/generate", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "1"})
	r.Header.Set(evergreen.HostHeader, "1")
	r.Header.Set(evergreen.HostSecretHeader, "secret")
	h := makeGenerateTasksPollHandler(sc, queue.NewLocalUnordered(1))

	assert.Error(t, h.Parse(ctx, r))
	task_1 := &task.Task{Id: "1"}
	assert.NoError(t, task_1.Insert())
	assert.Error(t, h.Parse(ctx, r))
	host_1 := &host.Host{Id: "1", Secret: "secret"}
	assert.NoError(t, host_1.Insert())
	assert.NoError(t, h.Parse(ctx, r))
}

func TestGeneratePollRun(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	sc := &data.MockConnector{}
	h := makeGenerateTasksPollHandler(sc, queue.NewLocalUnordered(1))

	impl, ok := h.(*generatePollHandler)
	assert.True(ok)
	impl.taskID = "0"
	resp := h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusInternalServerError, resp.Status())

	impl.taskID = "1"
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	assert.Equal(true, resp.Data().(*apimodels.GeneratePollResponse).Finished)

	impl.taskID = "2"
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	assert.Equal(false, resp.Data().(*apimodels.GeneratePollResponse).Finished)
}
