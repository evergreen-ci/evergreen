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
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	require.NoError(t, db.ClearCollections(task.Collection))
	task1 := task.Task{
		Id: "task1",
	}
	require.NoError(t, task1.Insert())
	h := &generateHandler{}
	h.taskID = "task1"
	r := h.Run(context.Background())
	assert.Equal(t, r.Data(), struct{}{})
	assert.Equal(t, r.Status(), http.StatusOK)
}

func TestGeneratePollParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	require.NoError(t, db.ClearCollections(task.Collection, host.Collection))

	sc := &data.DBConnector{}

	r, err := http.NewRequest("GET", "/task/1/generate", nil)
	require.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "1"})

	uri := "mongodb://localhost:27017"
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.Database("amboy_test").Drop(ctx))
	require.NotNil(t, env)
	q := env.RemoteQueueGroup()
	require.NotNil(t, q)

	h := makeGenerateTasksPollHandler(sc, q)
	require.NoError(t, h.Parse(ctx, r))
}

func TestGeneratePollRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	sc := &data.DBConnector{}
	require.NoError(t, db.ClearCollections(task.Collection))
	tasks := []task.Task{
		{
			Id:             "1",
			GeneratedTasks: true,
		},
		{
			Id: "2",
		},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	require.NotNil(t, env)
	q := env.RemoteQueueGroup()
	require.NotNil(t, q)

	h := makeGenerateTasksPollHandler(sc, q)

	impl, ok := h.(*generatePollHandler)
	require.True(t, ok)
	impl.taskID = "0"
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusInternalServerError, resp.Status())

	impl.taskID = "1"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status())
	require.Equal(t, true, resp.Data().(*apimodels.GeneratePollResponse).Finished)

	impl.taskID = "2"
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status())
	require.Equal(t, false, resp.Data().(*apimodels.GeneratePollResponse).Finished)
}
