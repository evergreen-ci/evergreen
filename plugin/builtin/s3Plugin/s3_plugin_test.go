package s3Plugin

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func reset(t *testing.T) {
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, artifact.Collection), t,
		"error clearing test collections")
}

func TestValidateS3BucketName(t *testing.T) {

	Convey("When validating s3 bucket names", t, func() {

		Convey("a bucket name that is too short should be rejected", func() {
			So(validateS3BucketName("a"), ShouldNotBeNil)
		})

		Convey("a bucket name that is too long should be rejected", func() {
			So(validateS3BucketName(strings.Repeat("a", 64)), ShouldNotBeNil)
		})

		Convey("a bucket name that does not start with a label should be rejected", func() {
			So(validateS3BucketName(".xx"), ShouldNotBeNil)
		})

		Convey("a bucket name that does not end with a label should be rejected", func() {
			So(validateS3BucketName("xx."), ShouldNotBeNil)
		})

		Convey("a bucket name with two consecutive periods should be rejected", func() {
			So(validateS3BucketName("xx..xx"), ShouldNotBeNil)
		})

		Convey("a bucket name with invalid characters should be rejected", func() {
			So(validateS3BucketName("a*a"), ShouldNotBeNil)
			So(validateS3BucketName("a'a"), ShouldNotBeNil)
			So(validateS3BucketName("a?a"), ShouldNotBeNil)
		})

		Convey("valid bucket names should be accepted", func() {
			So(validateS3BucketName("aaa"), ShouldBeNil)
			So(validateS3BucketName("aa.aa.aa"), ShouldBeNil)
			So(validateS3BucketName("a12.a-a"), ShouldBeNil)
		})
	})

}

func TestS3PutAndGet(t *testing.T) {
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, artifact.Collection), t,
		"error clearing test collections")

	conf := evergreen.TestConfig()
	testutil.ConfigureIntegrationTest(t, conf, "TestS3PutAndGet")

	Convey("When putting to and retrieving from an s3 bucket", t, func() {

		var putCmd *S3PutCommand
		var getCmd *S3GetCommand

		testDataDir := "testdata"
		remoteFile := "remote_mci_put_test.tgz"
		bucket := "mci-test-uploads"
		permissions := "private"
		contentType := "application/x-tar"
		displayName := "testfile"

		// create the local directory to be tarred
		localDirToTar := filepath.Join(testDataDir, "put_test")
		localFileToTar := filepath.Join(localDirToTar, "put_test_file.txt")
		testutil.HandleTestingErr(os.RemoveAll(localDirToTar), t, "Error removing"+
			" directory")
		testutil.HandleTestingErr(os.MkdirAll(localDirToTar, 0755), t,
			"Error creating directory")
		randStr := util.RandomString()
		So(ioutil.WriteFile(localFileToTar, []byte(randStr), 0755), ShouldBeNil)

		// tar it
		tarCmd := &command.LocalCommand{
			CmdString:        "tar czf put_test.tgz put_test",
			WorkingDirectory: testDataDir,
			Stdout:           ioutil.Discard,
			Stderr:           ioutil.Discard,
		}
		testutil.HandleTestingErr(tarCmd.Run(), t, "Error tarring directories")
		tarballSource := filepath.Join(testDataDir, "put_test.tgz")

		// remove the untarred version
		testutil.HandleTestingErr(os.RemoveAll(localDirToTar), t, "Error removing directories")

		Convey("the file retrieved should be the exact same as the file put", func() {

			// load params into the put command
			putCmd = &S3PutCommand{}
			putParams := map[string]interface{}{
				"aws_key":      conf.Providers.AWS.Id,
				"aws_secret":   conf.Providers.AWS.Secret,
				"local_file":   tarballSource,
				"remote_file":  remoteFile,
				"bucket":       bucket,
				"permissions":  permissions,
				"content_type": contentType,
				"display_name": displayName,
			}
			So(putCmd.ParseParams(putParams), ShouldBeNil)
			So(putCmd.Put(), ShouldBeNil)

			// next, get the file, untarring it
			getCmd = &S3GetCommand{}
			getParams := map[string]interface{}{
				"aws_key":      conf.Providers.AWS.Id,
				"aws_secret":   conf.Providers.AWS.Secret,
				"remote_file":  remoteFile,
				"bucket":       bucket,
				"extract_to":   testDataDir,
				"display_name": displayName,
			}
			So(getCmd.ParseParams(getParams), ShouldBeNil)
			So(getCmd.Get(), ShouldBeNil)
			// read in the file that was pulled down
			fileContents, err := ioutil.ReadFile(localFileToTar)
			So(err, ShouldBeNil)
			So(string(fileContents), ShouldEqual, randStr)

			// now, get the tarball without untarring it
			getCmd = &S3GetCommand{}
			localDlTarget := filepath.Join(testDataDir, "put_test_dl.tgz")
			getParams = map[string]interface{}{
				"aws_key":      conf.Providers.AWS.Id,
				"aws_secret":   conf.Providers.AWS.Secret,
				"remote_file":  remoteFile,
				"bucket":       bucket,
				"local_file":   localDlTarget,
				"display_name": displayName,
			}
			So(getCmd.ParseParams(getParams), ShouldBeNil)
			So(getCmd.Get(), ShouldBeNil)
			exists, err := util.FileExists(localDlTarget)
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)

		})

		Convey("the put command should always run if there is no variants filter", func() {
			// load params into the put command
			putCmd = &S3PutCommand{}
			putParams := map[string]interface{}{
				"aws_key":      conf.Providers.AWS.Id,
				"aws_secret":   conf.Providers.AWS.Secret,
				"local_file":   tarballSource,
				"remote_file":  remoteFile,
				"bucket":       bucket,
				"permissions":  permissions,
				"content_type": contentType,
				"display_name": displayName,
			}
			So(putCmd.ParseParams(putParams), ShouldBeNil)
			So(putCmd.shouldRunForVariant("linux-64"), ShouldBeTrue)
		})

		Convey("put cmd with variants filter should only run if variant is in list", func() {
			// load params into the put command
			putCmd = &S3PutCommand{}
			putParams := map[string]interface{}{
				"aws_key":        conf.Providers.AWS.Id,
				"aws_secret":     conf.Providers.AWS.Secret,
				"local_file":     tarballSource,
				"remote_file":    remoteFile,
				"bucket":         bucket,
				"permissions":    permissions,
				"content_type":   contentType,
				"display_name":   displayName,
				"build_variants": []string{"linux-64", "windows-64"},
			}
			So(putCmd.ParseParams(putParams), ShouldBeNil)

			So(putCmd.shouldRunForVariant("linux-64"), ShouldBeTrue)
			So(putCmd.shouldRunForVariant("osx-108"), ShouldBeFalse)
		})

		Convey("put cmd with 'optional' and missing file should not throw an error", func() {
			// load params into the put command
			putCmd = &S3PutCommand{}
			putParams := map[string]interface{}{
				"aws_key":      conf.Providers.AWS.Id,
				"aws_secret":   conf.Providers.AWS.Secret,
				"optional":     true,
				"local_file":   "this_file_does_not_exist.txt",
				"remote_file":  "remote_file",
				"bucket":       "test_bucket",
				"permissions":  "private",
				"content_type": "text/plain",
			}
			So(putCmd.ParseParams(putParams), ShouldBeNil)
			server, err := apiserver.CreateTestServer(conf, nil, plugin.APIPlugins, false)
			httpCom := plugintest.TestAgentCommunicator("testTask", "taskSecret", server.URL)
			pluginCom := &agent.TaskJSONCommunicator{"s3", httpCom}

			So(err, ShouldBeNil)

			err = putCmd.Execute(&plugintest.MockLogger{}, pluginCom,
				&model.TaskConfig{nil, nil, nil, nil, &model.BuildVariant{Name: "linux"}, &command.Expansions{}, "."}, make(chan bool))
			So(err, ShouldBeNil)
		})
	})

}
