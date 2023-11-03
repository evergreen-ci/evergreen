package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for abort task route

type TaskAbortSuite struct {
	suite.Suite
}

func TestTaskAbortSuite(t *testing.T) {
	suite.Run(t, new(TaskAbortSuite))
}

func (s *TaskAbortSuite) SetupSuite() {
	s.NoError(db.ClearCollections(task.Collection, user.Collection, build.Collection, serviceModel.VersionCollection))
	tasks := []task.Task{
		{Id: "task1", Status: evergreen.TaskStarted, BuildId: "b1", Version: "v1"},
		{Id: "task2", Status: evergreen.TaskStarted, BuildId: "b1", Version: "v1"},
	}
	s.NoError((&build.Build{Id: "b1"}).Insert())
	s.NoError((&serviceModel.Version{Id: "v1"}).Insert())
	u := &user.DBUser{Id: "user1"}
	s.NoError(u.Insert())
	for _, t := range tasks {
		s.NoError(t.Insert())
	}
}

func (s *TaskAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeTaskAbortHandler()
	rm.(*taskAbortHandler).taskId = "task1"
	res := rm.Run(ctx)

	s.Equal(http.StatusOK, res.Status())

	s.NotNil(res)
	tasks, err := task.Find(task.ByIds([]string{"task1", "task2"}))
	s.NoError(err)
	s.Equal("user1", tasks[0].ActivatedBy)
	s.Equal("", tasks[1].ActivatedBy)
	t, ok := res.Data().(*model.APITask)
	s.True(ok)
	s.Equal(utility.ToStringPtr("task1"), t.Id)

	res = rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)
	tasks, err = task.Find(task.ByIds([]string{"task1", "task2"}))
	s.NoError(err)
	s.Equal("user1", tasks[0].AbortInfo.User)
	s.Equal("", tasks[1].AbortInfo.User)
	t, ok = (res.Data()).(*model.APITask)
	s.True(ok)
	s.Equal(utility.ToStringPtr("task1"), t.Id)
}

func TestFetchArtifacts(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.ClearCollections(task.Collection, task.OldCollection, artifact.Collection))
	task1 := task.Task{
		Id:        "task1",
		Status:    evergreen.TaskSucceeded,
		Execution: 0,
	}
	assert.NoError(task1.Insert())
	assert.NoError(task1.Archive())
	entry := artifact.Entry{
		TaskId:          task1.Id,
		TaskDisplayName: "task",
		BuildId:         "b1",
		Execution:       1,
		Files: []artifact.File{
			{
				Name: "file1",
				Link: "l1",
			},
			{
				Name: "file2",
				Link: "l2",
			},
		},
	}
	assert.NoError(entry.Upsert())
	entry.Execution = 0
	assert.NoError(entry.Upsert())

	task2 := task.Task{
		Id:          "task2",
		Execution:   0,
		DisplayOnly: true,
		Status:      evergreen.TaskSucceeded,
	}
	assert.NoError(task2.Insert())
	assert.NoError(task2.Archive())

	taskGet := taskGetHandler{taskID: task1.Id}
	resp := taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(resp.Status(), http.StatusOK)
	apiTask := resp.Data().(*model.APITask)
	assert.Len(apiTask.Artifacts, 2)
	assert.Empty(apiTask.PreviousExecutions)

	// fetch all
	taskGet.fetchAllExecutions = true
	resp = taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(resp.Status(), http.StatusOK)
	apiTask = resp.Data().(*model.APITask)
	require.Len(apiTask.PreviousExecutions, 1)
	assert.NotZero(apiTask.PreviousExecutions[0])
	assert.NotEmpty(apiTask.PreviousExecutions[0].Artifacts)

	// fetchs a display task
	taskGet.taskID = "task2"
	taskGet.fetchAllExecutions = false
	resp = taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(resp.Status(), http.StatusOK)
	apiTask = resp.Data().(*model.APITask)
	assert.Empty(apiTask.PreviousExecutions)

	// fetch all, tasks with display tasks
	taskGet.fetchAllExecutions = true
	resp = taskGet.Run(context.Background())
	require.NotNil(resp)
	assert.Equal(resp.Status(), http.StatusOK)
	apiTask = resp.Data().(*model.APITask)
	require.Len(apiTask.PreviousExecutions, 1)
	assert.NotZero(apiTask.PreviousExecutions[0])
}

func TestGetDisplayTask(t *testing.T) {
	for testName, testCase := range map[string]func(context.Context, *testing.T){
		"SucceedsWithTaskInDisplayTask": func(ctx context.Context, t *testing.T) {
			tsk := task.Task{Id: "task_id"}
			displayTask := task.Task{
				Id:             "id",
				DisplayName:    "display_task_name",
				ExecutionTasks: []string{tsk.Id},
			}
			require.NoError(t, displayTask.Insert())

			h := makeGetDisplayTaskHandler()
			rh, ok := h.(*displayTaskGetHandler)
			require.True(t, ok)
			rh.taskID = tsk.Id
			require.NoError(t, tsk.Insert())

			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			info, ok := resp.Data().(*apimodels.DisplayTaskInfo)
			require.True(t, ok)
			assert.Equal(t, displayTask.Id, info.ID)
			assert.Equal(t, displayTask.DisplayName, info.Name)
		},
		"FailsWithNonexistentTask": func(ctx context.Context, t *testing.T) {
			h := makeGetDisplayTaskHandler()
			rh, ok := h.(*displayTaskGetHandler)
			require.True(t, ok)
			rh.taskID = "nonexistent"

			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"ReturnsOkIfNotPartOfDisplayTask": func(ctx context.Context, t *testing.T) {
			tsk := task.Task{Id: "task_id"}
			h := makeGetDisplayTaskHandler()
			require.NoError(t, tsk.Insert())
			rh, ok := h.(*displayTaskGetHandler)
			require.True(t, ok)
			rh.taskID = tsk.Id

			resp := rh.Run(ctx)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())
			info, ok := resp.Data().(*apimodels.DisplayTaskInfo)
			require.True(t, ok)
			assert.Zero(t, *info)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(task.Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(task.Collection))
			}()

			testCase(ctx, t)
		})
	}

}

func TestGetTaskSyncReadCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	creds := model.APIS3Credentials{
		Key:    utility.ToStringPtr("key"),
		Secret: utility.ToStringPtr("secret"),
		Bucket: utility.ToStringPtr("bucket"),
	}
	u := &user.DBUser{
		Id: evergreen.ParentPatchUser,
	}
	newSettings := &model.APIAdminSettings{
		ApiUrl:    utility.ToStringPtr("test"),
		ConfigDir: utility.ToStringPtr("test"),
		AuthConfig: &model.APIAuthConfig{
			Github: &model.APIGithubAuthConfig{
				Organization: utility.ToStringPtr("test"),
			},
		},
		Ui: &model.APIUIConfig{
			Secret:         utility.ToStringPtr("test"),
			Url:            utility.ToStringPtr("test"),
			DefaultProject: utility.ToStringPtr("test"),
		},
		Providers: &model.APICloudProviders{
			AWS: &model.APIAWSConfig{
				Pod: &model.APIAWSPodConfig{
					ECS: &model.APIECSConfig{},
				},
				TaskSyncRead: &creds,
			},
			Docker: &model.APIDockerConfig{
				APIVersion:    utility.ToStringPtr(""),
				DefaultDistro: utility.ToStringPtr(""),
			},
			GCE: &model.APIGCEConfig{
				ClientEmail:  utility.ToStringPtr("gce_email"),
				PrivateKey:   utility.ToStringPtr("gce_key"),
				PrivateKeyID: utility.ToStringPtr("gce_key_id"),
				TokenURI:     utility.ToStringPtr("gce_token"),
			},
			OpenStack: &model.APIOpenStackConfig{
				IdentityEndpoint: utility.ToStringPtr("endpoint"),
				Username:         utility.ToStringPtr("username"),
				Password:         utility.ToStringPtr("password"),
				DomainName:       utility.ToStringPtr("domain"),
				ProjectName:      utility.ToStringPtr("project"),
				ProjectID:        utility.ToStringPtr("project_id"),
				Region:           utility.ToStringPtr("region"),
			},
			VSphere: &model.APIVSphereConfig{
				Host:     utility.ToStringPtr("host"),
				Username: utility.ToStringPtr("vsphere"),
				Password: utility.ToStringPtr("vsphere_pass"),
			},
		},
	}
	_, err := data.SetEvergreenSettings(ctx, newSettings, &evergreen.Settings{}, u, true)
	require.NoError(t, err)
	rh := makeTaskSyncReadCredentialsGetHandler()
	defer cancel()
	resp := rh.Run(ctx)
	require.NotNil(t, resp)
	respCreds, ok := resp.Data().(evergreen.S3Credentials)
	require.True(t, ok)
	assert.Equal(t, *creds.Secret, respCreds.Secret)
	assert.Equal(t, *creds.Key, respCreds.Key)
	assert.Equal(t, *creds.Bucket, respCreds.Bucket)
}

func TestGetTaskSyncPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	expected := task.Task{
		Id:           "task_id",
		Project:      "project",
		Version:      "version",
		BuildVariant: "build_variant",
		DisplayName:  "name",
	}
	h := makeTaskSyncPathGetHandler()
	require.NoError(t, expected.Insert())
	rh, ok := h.(*taskSyncPathGetHandler)
	require.True(t, ok)
	rh.taskID = expected.Id

	defer cancel()
	resp := rh.Run(ctx)

	require.NotNil(t, resp)
	path, ok := resp.Data().(string)
	require.True(t, ok)
	assert.Equal(t, path, expected.S3Path(expected.BuildVariant, expected.DisplayName))
}

func TestGeneratedTasksGetHandler(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *generatedTasksGetHandler, generatorID string, generated []task.Task){
		"ReturnsGeneratedTasks": func(ctx context.Context, t *testing.T, rh *generatedTasksGetHandler, generatorID string, generated []task.Task) {
			for _, tsk := range generated {
				require.NoError(t, tsk.Insert())
			}
			rh.taskID = generatorID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			data := resp.Data()
			require.NotZero(t, data)
			taskInfos, ok := data.([]model.APIGeneratedTaskInfo)
			require.True(t, ok)

			require.Len(t, taskInfos, len(generated))
			for i := 0; i < len(generated); i++ {
				assert.Equal(t, generated[i].Id, taskInfos[i].TaskID)
				assert.Equal(t, generated[i].DisplayName, taskInfos[i].TaskName)
				assert.Equal(t, generated[i].BuildId, taskInfos[i].BuildID)
				assert.Equal(t, generated[i].BuildVariant, taskInfos[i].BuildVariant)
				assert.Equal(t, generated[i].BuildVariantDisplayName, taskInfos[i].BuildVariantDisplayName)
			}
		},
		"ReturnsErrorWithNoMatches": func(ctx context.Context, t *testing.T, rh *generatedTasksGetHandler, generatorID string, generated []task.Task) {
			rh.taskID = "nonexistent"

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(task.Collection))

			const generatorID = "generator"
			generated := []task.Task{
				{
					Id:                      "generated_task0",
					GeneratedBy:             generatorID,
					BuildId:                 "build_id0",
					BuildVariant:            "build-variant0",
					BuildVariantDisplayName: "first build variant",
				},
				{
					Id:                      "generated_task1",
					GeneratedBy:             generatorID,
					BuildId:                 "build_id1",
					BuildVariant:            "build-variant1",
					BuildVariantDisplayName: "second build variant",
				},
			}

			rh, ok := makeGetGeneratedTasks().(*generatedTasksGetHandler)
			require.True(t, ok)

			tCase(ctx, t, rh, generatorID, generated)
		})
	}
}
