package keyval_test

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/keyval"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func TestIncKey(t *testing.T) {
	Convey("With keyval plugin installed", t, func() {
		err := db.Clear(model.KeyValCollection)
		testutil.HandleTestingErr(err, t, "Couldn't clear test collection: %s", model.KeyValCollection)
		registry := plugin.NewSimpleRegistry()
		kvPlugin := &keyval.KeyValPlugin{}
		err = registry.Register(kvPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register keyval plugin")

		testConfig := testutil.TestConfig()

		server, err := service.CreateTestServer(testConfig, nil)
		testutil.HandleTestingErr(err, t, "couldn't create test server")

		configPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_keyval.yml")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "couldn't create test task")
		httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)
		logger := agentutil.NewTestLogger(slogger.StdOutAppender())

		Convey("Inc command should increment a key successfully", func() {
			taskConfig := modelData.TaskConfig
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, nil)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					for _, cmd := range pluginCmds {
						pluginCom := &comm.TaskJSONCommunicator{cmd.Plugin(), httpCom}
						err = cmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
						So(err, ShouldBeNil)
					}
				}
				So(taskConfig.Expansions.Get("testkey"), ShouldEqual, "2")
				So(taskConfig.Expansions.Get("testkey_x"), ShouldEqual, "1")
			}
		})
	})
}
