package agent

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPatchTask(t *testing.T) {
	setupTlsConfigs(t)
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	patchModes := []modelutil.PatchTestMode{modelutil.InlinePatch, modelutil.ExternalPatch}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchTask")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				Convey("With agent running a patched 'compile' "+tlsString, func() {
					for _, mode := range patchModes {
						Convey(fmt.Sprintf("Using patch mode %v", mode.String()), func() {
							modelData, err := modelutil.SetupAPITestData(testConfig, "compile", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), mode)
							githash := "1e5232709595db427893826ce19289461cba3f75"
							setupPatches(mode, modelData.Build, t,
								patchRequest{"", filepath.Join(testDirectory, "testdata/test.patch"), githash},
								patchRequest{"recursive", filepath.Join(testDirectory, "testdata/testmodule.patch"), githash})

							testutil.HandleTestingErr(err, t, "Error setting up test data: %v", err)
							testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
							testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
							defer testServer.Close()

							testAgent, err := createAgent(testServer, modelData.Host)
							testutil.HandleTestingErr(err, t, "failed to create agent: %v")
							defer testAgent.stop()

							So(assignAgentTask(testAgent, modelData.Task), ShouldBeNil)

							// actually run the task.
							// this function won't return until the whole thing is done.
							_, err = testAgent.RunTask()
							So(err, ShouldBeNil)
							time.Sleep(100 * time.Millisecond)
							testAgent.APILogger.FlushAndWait()
							printLogsForTask(modelData.Task.Id)

							Convey("all scripts in task should have been run successfully", func() {
								So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
								So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)

								So(scanLogsForTask(modelData.Task.Id, "", "Cloning into") || // git 1.8
									scanLogsForTask(modelData.Task.Id, "", "Initialized empty Git repository"), // git 1.7
									ShouldBeTrue)

								So(scanLogsForTask(modelData.Task.Id, "", "Fetching patch"), ShouldBeTrue)
								So(scanLogsForTask(modelData.Task.Id, "", "Applying patch with git..."), ShouldBeTrue)
								So(scanLogsForTask(modelData.Task.Id, "", "Applying module patch with git..."), ShouldBeTrue)

								So(scanLogsForTask(modelData.Task.Id, "", "i am compiling!"), ShouldBeTrue)
								So(scanLogsForTask(modelData.Task.Id, "", "i am sanity testing!"), ShouldBeTrue)

								testTask, err := task.FindOne(task.ById(modelData.Task.Id))
								testutil.HandleTestingErr(err, t, "Error finding test task: %v", err)
								So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)
							})
						})
					}
				})
			})
		}
	}

}
