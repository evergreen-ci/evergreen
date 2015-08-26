package s3copy_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/s3Plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/s3copy"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestS3CopyPluginExecution(t *testing.T) {

	testConfig := evergreen.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	testutil.ConfigureIntegrationTest(t, testConfig, "TestS3CopyPluginExecution")

	Convey("With a SimpleRegistry and test project file", t, func() {
		registry := plugin.NewSimpleRegistry()
		s3CopyPlugin := &S3CopyPlugin{}
		testutil.HandleTestingErr(registry.Register(s3CopyPlugin), t, "failed to register s3Copy plugin")
		testutil.HandleTestingErr(registry.Register(&s3Plugin.S3Plugin{}), t, "failed to register S3 plugin")
		testutil.HandleTestingErr(
			db.ClearCollections(model.PushlogCollection, version.Collection), t,
			"error clearing test collections")
		version := &version.Version{
			Id: "",
		}
		So(version.Insert(), ShouldBeNil)
		server, err := apiserver.CreateTestServer(evergreen.TestConfig(), nil, plugin.APIPlugins, false)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")

		httpCom := plugintest.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)

		//server.InstallPlugin(s3CopyPlugin)

		taskConfig, err := plugintest.CreateTestConfig("testdata/plugin_s3_copy.yml", t)
		testutil.HandleTestingErr(err, t, "failed to create test config: %v", err)
		taskConfig.WorkDir = "."
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		taskConfig.Expansions.Update(map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
		})

		Convey("the s3 copy command should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{s3CopyPlugin.Name(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig,
						make(chan bool))
					So(err, ShouldBeNil)
				}
			}
		})
	})
}
