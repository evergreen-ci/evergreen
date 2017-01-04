package agent

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAgentDebugHandler(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With an agent that has not been started", t, func() {
			testAgent, err := New(Options{
				TaskId: "task1",
			})
			So(err, ShouldBeNil)
			Convey("no task or command should be listed", func() {
				task, command := taskAndCommand(testAgent)
				So(task, ShouldEqual, "no running task")
				So(command, ShouldEqual, "no running command")
			})
		})
		Convey("With agent running a slow test and live API server over "+tlsString, t, func() {
			testTask, _, err := setupAPITestData(testConfig, "timeout_task", "linux-64",
				filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := createAgent(testServer, testTask)
			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			Convey("the agent should return the correct running task, command, and trace", func() {
				// run the slow task and take a debug trace during.
				var stack []byte
				var task, command string
				done := make(chan struct{})
				go func() {
					time.Sleep(time.Second)

					// this test is a racy because of how taskAndCommand works; and it doesn't
					// matter, because the agent is in the process of dying whenever this happens in
					// the wild and we wouldn't want to block during termination.

					testAgent.KillChan <- true
					task, command = taskAndCommand(testAgent)
					stack = util.DebugTrace()
					dumpToLogs(task, command, stack, testAgent)
					done <- struct{}{}
				}()
				testAgent.RunTask()
				testAgent.APILogger.Flush()
				<-done
				So(task, ShouldEqual, testTask.Id)
				So(command, ShouldEqual, "shell.exec")
				gcTesting := "testing.RunTests" // we know this will be present in the trace
				So(string(stack), ShouldContainSubstring, gcTesting)
				Convey("which should also be present in the logs", func() {
					So(scanLogsForTask(testTask.Id, "", gcTesting), ShouldBeTrue)
				})
			})
		})
	}
}
