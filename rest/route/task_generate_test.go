package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
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

func TestGenerateExecuteWithSmallFileInDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(task.Collection))

	tsk := task.Task{
		Id:      "task_id",
		Version: "version_id",
	}
	require.NoError(t, tsk.Insert())

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	genJSON := `{"key": "value"}`
	h := &generateHandler{
		env:    env,
		files:  []json.RawMessage{[]byte(genJSON)},
		taskID: tsk.Id,
	}
	r := h.Run(ctx)

	assert.Equal(t, r.Data(), struct{}{})
	assert.Equal(t, r.Status(), http.StatusOK)

	dbTask, err := task.FindOneId(tsk.Id)
	require.NoError(t, err)
	require.NotZero(t, dbTask)
	assert.Equal(t, dbTask.GeneratedJSONStorageMethod, evergreen.ProjectStorageMethodDB)

	genJSONInDB, err := task.GeneratedJSONFind(ctx, env.Settings(), dbTask)
	require.NoError(t, err)
	require.Len(t, genJSONInDB, len(h.files), "generated JSON in DB should be non-empty")
	assert.EqualValues(t, genJSON, genJSONInDB[0])

	queue, err := env.RemoteQueueGroup().Get(ctx, fmt.Sprintf("service.generate.tasks.version.%s", tsk.Version))
	require.NoError(t, err)
	stats := queue.Stats(ctx)
	assert.Equal(t, 1, stats.Total)
}

func TestGenerateExecuteWithLargeFileInS3(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings(), t.Name())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(c, pail.S3Options{
		Name:        ppConf.Bucket,
		Region:      endpoints.UsEast1RegionID,
		Credentials: pail.CreateAWSCredentials(ppConf.Key, ppConf.Secret, ""),
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))
	}()

	require.NoError(t, db.ClearCollections(task.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()

	tsk := task.Task{
		Id:      "task_id",
		Version: "version_id",
	}
	require.NoError(t, tsk.Insert())

	// Create string that is over the DB's 16 MB document limit to ensure it
	// gets stored in S3.
	genJSON := bytes.NewBufferString("{")
	for i := 0; i < 10e6; i++ {
		_, err := genJSON.WriteString(fmt.Sprintf(`"field-%d": "value-%d"`, i, i))
		require.NoError(t, err)
		if i < 10e6-1 {
			_, err := genJSON.WriteString(",")
			require.NoError(t, err)
		}
	}
	_, err = genJSON.WriteString("}")
	require.NoError(t, err)

	h := &generateHandler{
		env:    env,
		files:  []json.RawMessage{genJSON.Bytes()},
		taskID: tsk.Id,
	}
	r := h.Run(ctx)

	assert.Equal(t, r.Data(), struct{}{})
	assert.Equal(t, r.Status(), http.StatusOK)

	dbTask, err := task.FindOneId(tsk.Id)
	require.NoError(t, err)
	require.NotZero(t, dbTask)
	assert.Equal(t, dbTask.GeneratedJSONStorageMethod, evergreen.ProjectStorageMethodS3)

	genJSONInS3, err := task.GeneratedJSONFind(ctx, env.Settings(), dbTask)
	require.NoError(t, err)
	require.Len(t, genJSONInS3, len(h.files), "generated JSON in S3 should be non-empty")
	assert.EqualValues(t, genJSONInS3[0], genJSON.String())

	queue, err := env.RemoteQueueGroup().Get(ctx, fmt.Sprintf("service.generate.tasks.version.%s", tsk.Version))
	require.NoError(t, err)
	stats := queue.Stats(ctx)
	assert.Equal(t, 1, stats.Total)
}

func TestGeneratePollParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(task.Collection, host.Collection))
	r, err := http.NewRequest(http.MethodGet, "/task/1/generate", nil)
	require.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"task_id": "1"})

	h := makeGenerateTasksPollHandler()
	require.NoError(t, h.Parse(ctx, r))
}

func TestGeneratePollRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	h := makeGenerateTasksPollHandler()

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
