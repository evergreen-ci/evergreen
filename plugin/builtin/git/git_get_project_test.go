package git_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/plugin"
	. "10gen.com/mci/plugin/builtin/git"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func TestGitPlugin(t *testing.T) {
	Convey("With git plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		url, server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("mocktaskid", "mocktasksecret", url)

		server.InstallPlugin(gitPlugin)

		taskConfig, err := testutil.CreateTestConfig("testdata/plugin_clone.yml", t)
		util.HandleTestingErr(err, t, "failed to create test config")
		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(plugin, ShouldNotBeNil)
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(), httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
				}
			}
			err = os.RemoveAll(taskConfig.WorkDir)
			util.HandleTestingErr(err, t, "Couldn't clean up test temp dir")
		})
	})
}
