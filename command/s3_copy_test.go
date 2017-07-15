package s3copy_test

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/s3"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/s3copy"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	. "github.com/smartystreets/goconvey/convey"
)

func TestS3CopyPluginExecution(t *testing.T) {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	testutil.ConfigureIntegrationTest(t, testConfig, "TestS3CopyPluginExecution")

	Convey("With a SimpleRegistry and test project file", t, func() {
		registry := plugin.NewSimpleRegistry()
		s3CopyPlugin := &S3CopyPlugin{}
		testutil.HandleTestingErr(registry.Register(s3CopyPlugin), t, "failed to register s3Copy plugin")
		testutil.HandleTestingErr(registry.Register(&s3.S3Plugin{}), t, "failed to register S3 plugin")
		testutil.HandleTestingErr(
			db.ClearCollections(model.PushlogCollection, version.Collection), t,
			"error clearing test collections")
		version := &version.Version{
			Id: "versionId",
		}
		So(version.Insert(), ShouldBeNil)
		server, err := service.CreateTestServer(testConfig, nil)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		defer server.Close()

		pwd := testutil.GetDirectoryOfFile()
		configFile := filepath.Join(pwd, "testdata", "plugin_s3_copy.yml")
		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "linux-64", configFile, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")
		httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)
		taskConfig := modelData.TaskConfig
		taskConfig.WorkDir = "."

		logger := agentutil.NewTestLogger(slogger.StdOutAppender())
		taskConfig.Expansions.Update(map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
			"pwd":        pwd,
		})

		Convey("the s3 copy command should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &comm.TaskJSONCommunicator{s3CopyPlugin.Name(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig,
						make(chan bool))
					So(err, ShouldBeNil)
				}
			}
		})
	})
}
