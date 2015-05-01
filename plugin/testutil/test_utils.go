package testutil

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"testing"
	"time"
)

type MockLogger struct{}

func (self *MockLogger) LogLocal(level slogger.Level, messageFmt string,
	args ...interface{}) {
}

func (self *MockLogger) LogExecution(level slogger.Level, messageFmt string,
	args ...interface{}) {
}

func (self *MockLogger) LogTask(level slogger.Level, messageFmt string,
	args ...interface{}) {
}

func (self *MockLogger) LogSystem(level slogger.Level, messageFmt string,
	args ...interface{}) {
}

func (self *MockLogger) GetTaskLogWriter(level slogger.Level) io.Writer {
	return ioutil.Discard
}

func (self *MockLogger) Flush() {
	return
}

func CreateTestConfig(filename string, t *testing.T) (*model.TaskConfig, error) {
	util.HandleTestingErr(
		db.ClearCollections(model.TasksCollection, model.ProjectVarsCollection),
		t, "Failed to clear test data collection")

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	project := &model.Project{}
	err = yaml.Unmarshal(data, project)
	if err != nil {
		return nil, err
	}

	workDir, err := ioutil.TempDir("", "plugintest_")
	if err != nil {
		return nil, err
	}

	testTask := &model.Task{
		Id:           "mocktaskid",
		BuildId:      "testBuildId",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  "test",
		HostId:       "testHost",
		Secret:       "mocktasksecret",
		Status:       evergreen.TaskDispatched,
		Revision:     "d0c52298b222f4973c48e9834a57966c448547de",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	util.HandleTestingErr(testTask.Insert(), t, "failed to insert task")

	projectVars := &model.ProjectVars{
		Id: "mongodb-mongo-master",
		Vars: map[string]string{
			"abc": "xyz",
			"123": "456",
		},
	}
	_, err = projectVars.Upsert()
	util.HandleTestingErr(err, t, "failed to upsert project vars")
	return model.NewTaskConfig(&distro.Distro{Id: "linux-64"}, project,
		testTask, workDir)
}

func TestAgentCommunicator(taskId string, taskSecret string, apiRootUrl string) *agent.HTTPAgentCommunicator {
	httpCom, err := agent.NewHTTPAgentCommunicator(apiRootUrl, taskId, taskSecret, "")
	if err != nil {
		panic(err)
	}
	httpCom.MaxAttempts = 3
	httpCom.RetrySleep = 100 * time.Millisecond
	return httpCom
}

func SetupAPITestData(taskDisplayName string, isPatch bool, t *testing.T) (*model.Task, *build.Build, error) {
	//ignore errs here because the ns might just not exist.
	util.HandleTestingErr(
		db.ClearCollections(model.TasksCollection, build.Collection,
			host.Collection, version.Collection, patch.Collection),
		t, "Failed to clear test data collection")

	testHost := &host.Host{
		Id:          "testHost",
		Host:        "testHost",
		RunningTask: "testTaskId",
		StartedBy:   evergreen.User,
	}
	util.HandleTestingErr(testHost.Insert(), t, "failed to insert host")

	task := &model.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "rhel55",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Version:      "testVersionId",
		Secret:       "testTaskSecret",
		Status:       evergreen.TaskDispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
	}

	if isPatch {
		task.Requester = evergreen.PatchVersionRequester
	}

	util.HandleTestingErr(task.Insert(), t, "failed to insert task")

	version := &version.Version{Id: "testVersionId", BuildIds: []string{task.BuildId}}
	util.HandleTestingErr(version.Insert(), t, "failed to insert version %v")
	if isPatch {
		mainPatchContent, err := ioutil.ReadFile("testdata/test.patch")
		util.HandleTestingErr(err, t, "failed to read test patch file %v")
		modulePatchContent, err := ioutil.ReadFile("testdata/testmodule.patch")
		util.HandleTestingErr(err, t, "failed to read test module patch file %v")

		patch := &patch.Patch{
			Status:  evergreen.PatchCreated,
			Version: version.Id,
			Patches: []patch.ModulePatch{
				{
					ModuleName: "",
					Githash:    "d0c52298b222f4973c48e9834a57966c448547de",
					PatchSet:   patch.PatchSet{Patch: string(mainPatchContent)},
				},
				{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		util.HandleTestingErr(patch.Insert(), t, "failed to insert version %v")

	}

	session, _, err := db.GetGlobalSessionFactory().GetSession()
	util.HandleTestingErr(err, t, "couldn't get db session!")

	//Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).RemoveAll(bson.M{"t_id": task.Id})
	util.HandleTestingErr(err, t, "failed to remove logs")

	build := &build.Build{Id: "testBuildId", Tasks: []build.TaskCache{build.NewTaskCache(task.Id, task.DisplayName, true)}}

	util.HandleTestingErr(build.Insert(), t, "failed to insert build %v")
	return task, build, nil
}
