package s3

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
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

func TestS3PutAndGetSingleFile(t *testing.T) {
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
			_, err := putCmd.Put()
			So(err, ShouldBeNil)

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
			server, err := service.CreateTestServer(conf, nil, plugin.APIPlugins, false)
			httpCom := plugintest.TestAgentCommunicator("testTask", "taskSecret", server.URL)
			pluginCom := &comm.TaskJSONCommunicator{"s3", httpCom}

			So(err, ShouldBeNil)

			err = putCmd.Execute(&plugintest.MockLogger{}, pluginCom,
				&model.TaskConfig{nil, nil, nil, nil, nil, &model.BuildVariant{Name: "linux"}, &command.Expansions{}, "."}, make(chan bool))
			So(err, ShouldBeNil)
		})
		Convey("put cmd without 'optional' and missing file should throw an error", func() {
			// load params into the put command
			putCmd = &S3PutCommand{}
			putParams := map[string]interface{}{
				"aws_key":      conf.Providers.AWS.Id,
				"aws_secret":   conf.Providers.AWS.Secret,
				"local_file":   "this_file_does_not_exist.txt",
				"remote_file":  "remote_file",
				"bucket":       "test_bucket",
				"permissions":  "private",
				"content_type": "text/plain",
			}
			So(putCmd.ParseParams(putParams), ShouldBeNil)
			server, err := service.CreateTestServer(conf, nil, plugin.APIPlugins, false)
			httpCom := plugintest.TestAgentCommunicator("testTask", "taskSecret", server.URL)
			pluginCom := &comm.TaskJSONCommunicator{"s3", httpCom}

			So(err, ShouldBeNil)

			err = putCmd.Execute(&plugintest.MockLogger{}, pluginCom,
				&model.TaskConfig{nil, nil, nil, nil, nil, &model.BuildVariant{Name: "linux"}, &command.Expansions{}, "."}, make(chan bool))
			So(err, ShouldNotBeNil)
		})
	})
}

type multiPutFileInfo struct {
	remoteName  string
	localPath   string
	displayName string
	data        string
}

func TestS3PutAndGetMultiFile(t *testing.T) {
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, artifact.Collection), t,
		"error clearing test collections")

	conf := evergreen.TestConfig()
	testutil.ConfigureIntegrationTest(t, conf, "TestS3PutAndGet")

	Convey("When putting to and retrieving from an s3 bucket", t, func() {

		testDataDir := "testdata"
		bucket := "mci-test-uploads"
		permissions := "private"
		contentType := "application/x-tar"
		remoteFilePrefix := "remote_mci_put_test-"
		displayNamePrefix := "testfile-"

		// create the local directory
		localDir := filepath.Join(testDataDir, "put_test")
		localFilePrefix := filepath.Join(localDir, "put_test_file")
		defer func() {
			testutil.HandleTestingErr(os.RemoveAll(testDataDir), t, "error removing test dir")
		}()
		testutil.HandleTestingErr(os.RemoveAll(localDir), t,
			"Error removing directory")
		testutil.HandleTestingErr(os.MkdirAll(localDir, 0755), t,
			"Error creating directory")
		fileInfos := []multiPutFileInfo{}
		for i := 0; i < 5; i++ {
			randStr := util.RandomString()
			localPath := fmt.Sprintf("%s%d.txt", localFilePrefix, i)
			localName := filepath.Base(localPath)
			remoteName := fmt.Sprintf("%s%s", remoteFilePrefix, localName)
			displayName := fmt.Sprintf("%s%s", displayNamePrefix, localName)
			So(ioutil.WriteFile(localPath, []byte(randStr), 0755), ShouldBeNil)
			fileInfo := multiPutFileInfo{
				remoteName:  remoteName,
				localPath:   localPath,
				displayName: displayName,
				data:        randStr,
			}
			fileInfos = append(fileInfos, fileInfo)
		}

		Convey("the files should be put without error", func() {
			includesFilter := []string{fmt.Sprintf("%s/*.txt", localDir)}

			// load params into the put command
			putCmd := &S3PutCommand{}
			putParams := map[string]interface{}{
				"aws_key":                    conf.Providers.AWS.Id,
				"aws_secret":                 conf.Providers.AWS.Secret,
				"local_files_include_filter": includesFilter,
				"remote_file":                remoteFilePrefix,
				"bucket":                     bucket,
				"permissions":                permissions,
				"content_type":               contentType,
				"display_name":               displayNamePrefix,
			}
			So(putCmd.ParseParams(putParams), ShouldBeNil)
			_, err := putCmd.Put()
			So(err, ShouldBeNil)
			Convey("the files should each be gotten without error", func() {
				for _, f := range fileInfos {
					// next, get the file
					getCmd := &S3GetCommand{}
					getParams := map[string]interface{}{
						"aws_key":      conf.Providers.AWS.Id,
						"aws_secret":   conf.Providers.AWS.Secret,
						"remote_file":  f.remoteName,
						"bucket":       bucket,
						"local_file":   f.localPath,
						"display_name": f.displayName,
					}
					So(getCmd.ParseParams(getParams), ShouldBeNil)
					So(getCmd.Get(), ShouldBeNil)

					// read in the file that was pulled down
					fileContents, err := ioutil.ReadFile(f.localPath)
					So(err, ShouldBeNil)
					So(string(fileContents), ShouldEqual, f.data)

				}
			})
		})

	})
}

func TestAttachResults(t *testing.T) {
	remoteFname := "remote_file"
	localFpath := "path/to/local_file"
	taskId := "testTask"
	displayName := "display_name"
	testBucket := "testBucket"
	Convey("When putting to an s3 bucket", t, func() {
		testutil.HandleTestingErr(
			db.ClearCollections(task.Collection, artifact.Collection), t,
			"error clearing test collections")

		testTask := &task.Task{
			Id: taskId,
		}
		testTask.Insert()

		conf := evergreen.TestConfig()
		testutil.ConfigureIntegrationTest(t, conf, "TestAttachResults")
		server, err := service.CreateTestServer(conf, nil, plugin.APIPlugins, false)
		So(err, ShouldBeNil)

		httpCom := plugintest.TestAgentCommunicator(taskId, "taskSecret", server.URL)
		pluginCom := &comm.TaskJSONCommunicator{"s3", httpCom}

		s3pc := S3PutCommand{
			LocalFile:   localFpath,
			RemoteFile:  remoteFname,
			DisplayName: displayName,
			Visibility:  "visible",
			Bucket:      testBucket,
		}
		Convey("and attach is multi", func() {
			s3pc.LocalFilesIncludeFilter = []string{"one", "two"}
			s3pc.AttachTaskFiles(&plugintest.MockLogger{}, pluginCom,
				s3pc.LocalFile, s3pc.RemoteFile)
			Convey("files should each be added properly", func() {
				entry, err := artifact.FindOne(artifact.ByTaskId(taskId))
				So(err, ShouldBeNil)
				So(len(entry.Files), ShouldEqual, 1)
				file := entry.Files[0]

				So(file.Link, ShouldEqual, fmt.Sprintf("%s%s/%s%s", s3baseURL, testBucket, remoteFname, filepath.Base(localFpath)))
				So(file.Name, ShouldEqual, fmt.Sprintf("%s%s", displayName, filepath.Base(localFpath)))
			})
		})
		Convey("and attaching is singular", func() {
			s3pc.LocalFilesIncludeFilter = []string{}
			s3pc.AttachTaskFiles(&plugintest.MockLogger{}, pluginCom,
				s3pc.LocalFile, s3pc.RemoteFile)
			Convey("file should be added properly", func() {
				entry, err := artifact.FindOne(artifact.ByTaskId(taskId))
				So(err, ShouldBeNil)
				So(len(entry.Files), ShouldEqual, 1)
				file := entry.Files[0]

				So(file.Link, ShouldEqual, fmt.Sprintf("%s%s/%s", s3baseURL, testBucket, remoteFname))
				So(file.Name, ShouldEqual, fmt.Sprintf("%s", displayName))
			})
		})
		Convey("and attach is called many times", func() {
			filesList := make([][]string, 10)
			for i := 0; i < 10; i++ {
				filesList[i] = []string{fmt.Sprintf("remote%d-", i), fmt.Sprintf("local%d", i)}
			}
			s3pc.LocalFilesIncludeFilter = []string{"one", "two"}
			for _, fileData := range filesList {
				s3pc.AttachTaskFiles(&plugintest.MockLogger{}, pluginCom,
					fileData[1], fileData[0])
			}
			Convey("files should each be added properly", func() {
				entry, err := artifact.FindOne(artifact.ByTaskId(taskId))
				So(err, ShouldBeNil)
				So(len(entry.Files), ShouldEqual, 10)
				for _, file := range entry.Files {
					entryIndex := fetchFileIndex(file.Name, displayName, filesList)
					So(entryIndex, ShouldNotEqual, -1)
					So(file.Link, ShouldEqual, fmt.Sprintf("%s%s/%s%s", s3baseURL,
						testBucket, filesList[entryIndex][0], filesList[entryIndex][1]))
					So(file.Name, ShouldEqual, fmt.Sprintf("%s%s", displayName,
						filesList[entryIndex][1]))
					filesList = append(filesList[:entryIndex], filesList[entryIndex+1:]...)
				}
				So(len(filesList), ShouldEqual, 0)
			})
		})
	})
}

func fetchFileIndex(fName, displayName string, filesList [][]string) int {
	for index, fileData := range filesList {
		fullDisplayName := fmt.Sprintf("%s%s", displayName, fileData[1])
		if fName == fullDisplayName {
			return index
		}
	}
	return -1
}
