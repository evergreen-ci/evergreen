package agent

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/goamz/goamz/aws"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPushTask(t *testing.T) {
	testConfig := evergreen.TestConfig()
	setupTlsConfigs(t)
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPushTask")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				Convey("With agent running a push task "+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, evergreen.PushStage,
						"linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
					testutil.HandleTestingErr(err, t, "Error setting up test data: %v", err)
					testutil.HandleTestingErr(db.ClearCollections(artifact.Collection), t, "can't clear files collection")
					testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Api.HttpsCert)
					testutil.HandleTestingErr(err, t, "Error making test agent: %v", err)

					// actually run the task.
					// this function won't return until the whole thing is done.
					testAgent.RunTask()
					time.Sleep(100 * time.Millisecond)
					testAgent.APILogger.FlushAndWait()
					printLogsForTask(testTask.Id)
					newDate := testAgent.taskConfig.Expansions.Get("new_date")

					Convey("all scripts in task should have been run successfully", func() {
						So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)

						So(scanLogsForTask(testTask.Id, "", "push task pre-run!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "", "push task post-run!"), ShouldBeTrue)

						Convey("s3.put attaches task file properly", func() {
							entry, err := artifact.FindOne(artifact.ByTaskId(testTask.Id))
							So(err, ShouldBeNil)
							So(len(entry.Files), ShouldEqual, 2)
							for _, element := range entry.Files {
								So(element.Name, ShouldNotEqual, "")
							}
							So(entry.Files[0].Name, ShouldEqual, "push_file")
							link := "https://s3.amazonaws.com/build-push-testing/pushtest-stage/unittest-testTaskId-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP.txt"
							So(entry.Files[0].Link, ShouldEqual, link)
						})
						Convey("s3.copy attached task file properly", func() {
							entry, err := artifact.FindOne(artifact.ByTaskId(testTask.Id))
							So(err, ShouldBeNil)
							So(len(entry.Files), ShouldNotEqual, 0)
							So(entry.Files[0].Name, ShouldEqual, "push_file")
							So(entry.Files[1].Name, ShouldEqual, "copy_file")
							So(entry.Files[0].Link, ShouldEqual, "https://s3.amazonaws.com/build-push-testing/pushtest-stage/unittest-testTaskId-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP.txt")
							So(entry.Files[1].Link, ShouldEqual,
								"https://s3.amazonaws.com/build-push-testing/pushtest/unittest-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP-latest.txt")
						})

						testTask, err = task.FindOne(task.ById(testTask.Id))
						testutil.HandleTestingErr(err, t, "Error finding test task: %v", err)
						So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)

						// Check the file written to s3 is what we expected
						auth := &aws.Auth{
							AccessKey: testConfig.Providers.AWS.Id,
							SecretKey: testConfig.Providers.AWS.Secret,
						}

						// check the staging location first
						filebytes, err := getS3FileBytes(auth, "build-push-testing", "/pushtest-stage/unittest-testTaskId-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP.txt")
						testutil.HandleTestingErr(err, t, "Failed to get file from s3: %v", err)
						So(string(filebytes), ShouldEqual, newDate+"\n")

						// now check remote location (after copy)
						filebytes, err = getS3FileBytes(auth, "build-push-testing", "/pushtest/unittest-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP-latest.txt")

						testutil.HandleTestingErr(err, t, "Failed to get remote file from s3: %v", err)
						So(string(filebytes), ShouldEqual, newDate+"\n")
					})
				})
			})
		}
	}
}

func getS3FileBytes(auth *aws.Auth, bucket string, path string) ([]byte, error) {
	session := thirdparty.NewS3Session(auth, aws.USEast)
	s3bucket := session.Bucket(bucket)
	reader, err := s3bucket.GetReader(path)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(reader)
}
