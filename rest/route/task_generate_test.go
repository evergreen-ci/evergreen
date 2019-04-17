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
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
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
	h := &generateHandler{sc: &data.MockConnector{}}
	r := h.Run(context.Background())
	assert.Equal(t, r.Data(), struct{}{})
	assert.Equal(t, r.Status(), http.StatusOK)
}

func localConstructor(ctx context.Context) (amboy.Queue, error) {
	return queue.NewLocalUnordered(1), nil
}

func remoteConstructor(ctx context.Context) (queue.Remote, error) {
	return queue.NewRemoteUnordered(1), nil
}

func TestGeneratePollParse(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, host.Collection))

	sc := &data.MockConnector{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := http.NewRequest("GET", "/task/1/generate", nil)
	require.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "1"})
	r.Header.Set(evergreen.HostHeader, "1")
	r.Header.Set(evergreen.HostSecretHeader, "secret")

	uri := "mongodb://localhost:27017"
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.Database("amboy_test").Drop(ctx))
	opts := queue.RemoteQueueGroupOptions{
		Client:      client,
		Constructor: remoteConstructor,
		MongoOptions: queue.MongoDBOptions{
			URI: uri,
			DB:  "amboy_test",
		},
		Prefix: "gen",
	}
	q, err := queue.NewRemoteQueueGroup(ctx, opts)
	require.NoError(t, err)

	h := makeGenerateTasksPollHandler(sc, q)
	require.Error(t, h.Parse(ctx, r))
	task_1 := &task.Task{Id: "1"}
	require.NoError(t, task_1.Insert())
	require.Error(t, h.Parse(ctx, r))
	host_1 := &host.Host{Id: "1", Secret: "secret"}
	require.NoError(t, host_1.Insert())
	require.NoError(t, h.Parse(ctx, r))
}

func TestGeneratePollRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sc := &data.MockConnector{}

	uri := "mongodb://localhost:27017"
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.Database("amboy_test").Drop(ctx))
	opts := queue.RemoteQueueGroupOptions{
		Client:      client,
		Constructor: remoteConstructor,
		MongoOptions: queue.MongoDBOptions{
			URI: uri,
			DB:  "amboy_test",
		},
		Prefix: "gen",
	}
	q, err := queue.NewRemoteQueueGroup(ctx, opts)
	require.NoError(t, err)

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
