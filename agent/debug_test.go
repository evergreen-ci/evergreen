package agent

import (
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"strings"
	"testing"
	"time"
)

func TestAgentDebugHandler(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With an agent that has not been started", t, func() {
			testAgent, err := New("", "task1", "task1", "", testConfig.Api.HttpsCert)
			So(err, ShouldBeNil)
			Convey("no task or command should be listed", func() {
				task, command := taskAndCommand(testAgent)
				So(task, ShouldEqual, "no running task")
				So(command, ShouldEqual, "no running command")
			})
		})
		Convey("With agent running a slow test and live API server over "+tlsString, t, func() {
			testTask, _, err := setupAPITestData(testConfig, "timeout_task", "linux-64", "testdata/config_test_plugin/project/evergreen-ci-render.yml", NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Api.HttpsCert)
			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			Convey("the agent should return the correct running task, command, and trace", func() {
				// run the slow task and take a debug trace during.
				go func() {
					time.Sleep(time.Second)
					task, command := taskAndCommand(testAgent)
					So(task, ShouldEqual, testTask.Id)
					So(command, ShouldEqual, "shell.exec")
					stack := trace()
					So(strings.Index(string(stack), "(*ShellExecCommand).Execute"), ShouldBeGreaterThan, 0)
					So(strings.Index(string(stack), "(*Agent).RunTask"), ShouldBeGreaterThan, 0)
					dumpToLogs(task, command, stack, testAgent)
				}()
				testAgent.RunTask()
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				Convey("which should also be present in the logs", func() {
					So(scanLogsForTask(testTask.Id, "(*ShellExecCommand).Execute"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "(*Agent).RunTask"), ShouldBeTrue)
				})
			})
		})
	}
}
