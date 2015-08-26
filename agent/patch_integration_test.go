package agent

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestPatchTask(t *testing.T) {
	setupTlsConfigs(t)
	testConfig := evergreen.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	patchModes := []patchTestMode{InlinePatch, ExternalPatch}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchTask")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				Convey("With agent running a patched 'compile'"+tlsString, func() {
					for _, mode := range patchModes {
						Convey(fmt.Sprintf("Using patch mode %v", mode.String()), func() {
							testTask, _, err := setupAPITestData(testConfig, "compile", "linux-64", mode, t)
							testutil.HandleTestingErr(err, t, "Error setting up test data: %v", err)
							testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
							testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
							testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Expansions["api_httpscert"])

							// actually run the task.
							// this function won't return until the whole thing is done.
							testAgent.RunTask()
							time.Sleep(100 * time.Millisecond)
							testAgent.APILogger.FlushAndWait()
							printLogsForTask(testTask.Id)

							Convey("all scripts in task should have been run successfully", func() {
								So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

								So(scanLogsForTask(testTask.Id, "Cloning into") || // git 1.8
									scanLogsForTask(testTask.Id, "Initialized empty Git repository"), // git 1.7
									ShouldBeTrue)

								So(scanLogsForTask(testTask.Id, "i am patched!"), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "i am a patched module"), ShouldBeTrue)

								So(scanLogsForTask(testTask.Id, "i am compiling!"), ShouldBeTrue)
								So(scanLogsForTask(testTask.Id, "i am sanity testing!"), ShouldBeTrue)

								testTask, err = model.FindTask(testTask.Id)
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
