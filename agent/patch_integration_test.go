package agent

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutils"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"
)

func TestPatchTask(t *testing.T) {
	setupTlsConfigs(t)
	testConfig := evergreen.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	testutils.ConfigureIntegrationTest(t, testConfig, "TestPatchTask")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				configAbsPath, err := filepath.Abs(testSetup.configPath)
				util.HandleTestingErr(err, t, "Couldn't get abs path for config: %v", err)

				Convey("With agent running a patched 'compile'"+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, evergreen.CompileStage,
						"linux-64", true, t)
					util.HandleTestingErr(err, t, "Error setting up test data: %v", err)

					testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
					util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := NewAgent(testServer.URL, testTask.Id,
						testTask.Secret,
						Verbose, testConfig.Expansions["api_httpscert"])

					//actually run the task.
					//this function won't return until the whole thing is done.
					workDir, err := ioutil.TempDir("", "mci_testtask_")
					util.HandleTestingErr(err, t, "Error creating temp data: %v", err)
					RunTask(testAgent, configAbsPath, workDir)
					time.Sleep(100 * time.Millisecond)
					testAgent.RemoteAppender.FlushAndWait()
					printLogsForTask(testTask.Id)

					Convey("all scripts in task should have been run successfully", func() {
						So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

						So(scanLogsForTask(testTask.Id, "Cloning into") || // git 1.8
							scanLogsForTask(testTask.Id, "Initialized empty Git repository"), // git 1.7
							ShouldBeTrue)

						So(scanLogsForTask(testTask.Id, "i am patched!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "i am a patched module"), ShouldBeTrue)

						So(scanLogsForTask(testTask.Id, "i am compiling!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "i am sanity testing!"), ShouldBeTrue)

						testTask, err = model.FindTask(testTask.Id)
						util.HandleTestingErr(err, t, "Error finding test task: %v", err)
						So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)
					})
				})
			})
		}
	}

}
