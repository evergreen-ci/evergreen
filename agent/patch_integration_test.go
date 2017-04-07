package agent

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
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
	patchModes := []patchTestMode{InlinePatch, ExternalPatch}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchTask")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				Convey("With agent running a patched 'compile' "+tlsString, func() {
					for _, mode := range patchModes {
						Convey(fmt.Sprintf("Using patch mode %v", mode.String()), func() {
							testTask, b, h, err := setupAPITestData(testConfig, "compile", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), mode, t)

							githash := "1e5232709595db427893826ce19289461cba3f75"
							setupPatches(mode, b, t,
								patchRequest{"", filepath.Join(testDirectory, "testdata/test.patch"), githash},
								patchRequest{"recursive", filepath.Join(testDirectory, "testdata/testmodule.patch"), githash})

							testutil.HandleTestingErr(err, t, "Error setting up test data: %v", err)
							testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
							testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
							defer testServer.Close()

							testAgent, err := createAgent(testServer, testTask, h)
							testutil.HandleTestingErr(err, t, "failed to create agent: %v")
							defer testAgent.stop()

							// actually run the task.
							// this function won't return until the whole thing is done.
							_, err = testAgent.RunTask()
							So(err, ShouldBeNil)
							time.Sleep(100 * time.Millisecond)
							testAgent.APILogger.FlushAndWait()
							printLogsForTask(testTask.Id)

							Convey("all scripts in task should have been run successfully", func() {
								So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)

								So(scanLogsForTask(testTask.Id, "", "Cloning into") || // git 1.8
									scanLogsForTask(testTask.Id, "", "Initialized empty Git repository"), // git 1.7
									ShouldBeTrue)

								So(scanLogsForTask(testTask.Id, "", "Fetching patch"), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "", "Applying patch with git..."), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "", "Applying module patch with git..."), ShouldBeTrue)

								So(scanLogsForTask(testTask.Id, "", "i am compiling!"), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "", "i am sanity testing!"), ShouldBeTrue)

								testTask, err = task.FindOne(task.ById(testTask.Id))
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
