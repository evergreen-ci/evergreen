package shell

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestShellExecuteCommand(t *testing.T) {
	stopper := make(chan bool)
	defer close(stopper)

	testConfig := testutil.TestConfig()
	server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
	if err != nil {
		t.Fatalf("failed to create test server %+v", err)
	}
	defer server.Close()

	httpCom := plugintest.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)
	jsonCom := &comm.TaskJSONCommunicator{"shell", httpCom}

	conf := &model.TaskConfig{Expansions: &command.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}

	Convey("With a shell command", t, func() {

		Convey("if unset, default is determined by local command", func() {
			cmd := &ShellExecCommand{}
			So(cmd.Execute(&plugintest.MockLogger{}, jsonCom, conf, stopper), ShouldBeNil)
			So(cmd.Shell, ShouldEqual, "")
		})

		shells := []string{"bash", "python", "sh"}

		if runtime.GOOS != "windows" {
			shells = append(shells, "/bin/sh", "/bin/bash", "/usr/bin/python")
		}

		for _, sh := range shells {
			Convey(fmt.Sprintf("when set, %s is not overwritten during execution", sh), func() {
				cmd := &ShellExecCommand{Shell: sh}
				So(cmd.Execute(&plugintest.MockLogger{}, jsonCom, conf, stopper), ShouldBeNil)
				So(cmd.Shell, ShouldEqual, sh)
			})
		}
	})
}
