package agent

import (
	"10gen.com/mci"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/artifact"
	"10gen.com/mci/plugin"
	"10gen.com/mci/testutils"
	"10gen.com/mci/util"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"
)

func TestPushTask(t *testing.T) {
	testConfig := mci.TestConfig()
	setupTlsConfigs(t)
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	testutils.ConfigureIntegrationTest(t, testConfig, "TestPushTask")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				configAbsPath, err := filepath.Abs(testSetup.configPath)
				util.HandleTestingErr(err, t, "Couldn't get abs path for config: %v", err)

				Convey("With agent running a push task "+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, mci.PushStage,
						"linux-64", false, t)
					util.HandleTestingErr(err, t, "Error setting up test data: %v", err)
					util.HandleTestingErr(db.ClearCollections(artifact.Collection), t, "can't clear files collection")

					testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
					util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
						Verbose, testConfig.Expansions["api_httpscert"])
					util.HandleTestingErr(err, t, "Error making test agent: %v", err)

					//actually run the task.
					//this function won't return until the whole thing is done.
					workDir, err := ioutil.TempDir("", "testtask_")
					util.HandleTestingErr(err, t, "Error creating temp data: %v", err)
					RunTask(testAgent, configAbsPath, workDir)
					time.Sleep(100 * time.Millisecond)
					testAgent.RemoteAppender.FlushAndWait()
					printLogsForTask(testTask.Id)
					newDate := testAgent.taskConfig.Expansions.Get("new_date")

					Convey("all scripts in task should have been run successfully", func() {
						So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

						So(scanLogsForTask(testTask.Id, "push task pre-run!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "push task post-run!"), ShouldBeTrue)

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

						testTask, err = model.FindTask(testTask.Id)
						util.HandleTestingErr(err, t, "Error finding test task: %v", err)
						So(testTask.Status, ShouldEqual, mci.TaskSucceeded)

						//Check the file written to s3 is what we expected
						auth := &aws.Auth{
							AccessKey: testConfig.Providers.AWS.Id,
							SecretKey: testConfig.Providers.AWS.Secret,
						}

						// check the staging location first
						filebytes, err := getS3FileBytes(auth, "build-push-testing", "/pushtest-stage/unittest-testTaskId-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP.txt")
						util.HandleTestingErr(err, t, "Failed to get file from s3: %v", err)
						So(string(filebytes), ShouldEqual, newDate+"\n")

						// now check remote location (after copy)
						filebytes, err = getS3FileBytes(auth, "build-push-testing", "/pushtest/unittest-DISTRO_EXP-BUILDVAR_EXP-FILE_EXP-latest.txt")

						util.HandleTestingErr(err, t, "Failed to get remote file from s3: %v", err)
						So(string(filebytes), ShouldEqual, newDate+"\n")
					})
				})
			})
		}
	}
}

func getS3FileBytes(auth *aws.Auth, bucket string, path string) ([]byte, error) {
	session := s3.New(*auth, aws.USEast)
	s3bucket := session.Bucket(bucket)
	reader, err := s3bucket.GetReader(path)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(reader)
}
