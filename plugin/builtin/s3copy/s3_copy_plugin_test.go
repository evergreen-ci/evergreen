package s3copy_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"10gen.com/mci/plugin/builtin/s3Plugin"
	. "10gen.com/mci/plugin/builtin/s3copy"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/testutils"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestS3CopyPluginExecution(t *testing.T) {

	testConfig := mci.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	testutils.ConfigureIntegrationTest(t, testConfig, "TestS3CopyPluginExecution")

	Convey("With a SimpleRegistry and test project file", t, func() {
		registry := plugin.NewSimpleRegistry()
		s3CopyPlugin := &S3CopyPlugin{}
		util.HandleTestingErr(registry.Register(s3CopyPlugin), t, "failed to register s3Copy plugin")
		util.HandleTestingErr(registry.Register(s3Plugin.S3Plugin{}), t, "failed to register S3 plugin")
		util.HandleTestingErr(
			db.ClearCollections(model.PushlogCollection, model.VersionsCollection), t,
			"error clearing test collections")
		version := &model.Version{
			Id: "",
		}
		So(version.Insert(), ShouldBeNil)
		url, server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")

		httpCom := testutil.TestAgentCommunicator("mocktaskid", "mocktasksecret", url)

		server.InstallPlugin(s3CopyPlugin)

		taskConfig, err := testutil.CreateTestConfig("testdata/plugin_s3_copy.yml", t)
		util.HandleTestingErr(err, t, "failed to create test config: %v", err)
		taskConfig.WorkDir = "."
		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		taskConfig.Expansions.Update(map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
		})

		Convey("the s3 copy command should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin "+
						"command: %v")
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(),
						httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig,
						make(chan bool))
					So(err, ShouldBeNil)
				}
			}
		})
	})
}
