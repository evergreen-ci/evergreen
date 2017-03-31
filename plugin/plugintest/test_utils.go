package plugintest

import (
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/model/patch"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
)

type MockLogger struct{}

func (_ *MockLogger) Flush()                                                                   {}
func (_ *MockLogger) LogLocal(level slogger.Level, messageFmt string, args ...interface{})     {}
func (_ *MockLogger) LogExecution(level slogger.Level, messageFmt string, args ...interface{}) {}
func (_ *MockLogger) LogTask(level slogger.Level, messageFmt string, args ...interface{})      {}
func (_ *MockLogger) LogSystem(level slogger.Level, messageFmt string, args ...interface{})    {}
func (_ *MockLogger) GetTaskLogWriter(level slogger.Level) io.Writer                           { return ioutil.Discard }
func (_ *MockLogger) GetSystemLogWriter(level slogger.Level) io.Writer                         { return ioutil.Discard }

func TestAgentCommunicator(testData *modelutil.TestModelData, apiRootUrl string) *comm.HTTPCommunicator {
	hostId := ""
	hostSecret := ""
	if testData.Host != nil {
		hostId = testData.Host.Id
		hostSecret = testData.Host.Secret
	}
	agentCommunicator, err := comm.NewHTTPCommunicator(apiRootUrl, hostId, hostSecret, "", nil)
	if err != nil {
		panic(err)
	}
	agentCommunicator.MaxAttempts = 3
	agentCommunicator.RetrySleep = 100 * time.Millisecond

	if testData.Task != nil {
		agentCommunicator.TaskId = testData.Task.Id
		agentCommunicator.TaskSecret = testData.Task.Secret
	}
	return agentCommunicator
}

func SetupPatchData(apiData *modelutil.TestModelData, patchPath string, t *testing.T) error {

	if patchPath != "" {
		modulePatchContent, err := ioutil.ReadFile(patchPath)
		testutil.HandleTestingErr(err, t, "failed to read test module patch file %v")

		patch := &patch.Patch{
			Status:  evergreen.PatchCreated,
			Version: apiData.TaskConfig.Version.Id,
			Patches: []patch.ModulePatch{
				{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		testutil.HandleTestingErr(patch.Insert(), t, "failed to insert patch %v")

	}

	return nil
}
