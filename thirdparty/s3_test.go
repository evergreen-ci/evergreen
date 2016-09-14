package thirdparty

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	sourceURL     = "s3://build-push-testing/test/source/testfile"
	destUrl       = "s3://build-push-testing/test/dest/testfile"
	testURLBucket = "build-push-testing"
	testURLPath   = "/test/push/path/mongodb-pushname-pusharch-latest.tgz"
	testURL       = "s3://s3_key:s3_secret@build-push-testing/test/" +
		"push/path/mongodb-pushname-pusharch-latest.tgz"
)

func TestS3ParseUrl(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestS3ParseUrl")
	Convey("When given an S3 location to parse...", t, func() {
		Convey("the bucket and path should be parsed correctly", func() {
			bucket, path, err := GetS3Location(testURL)
			So(err, ShouldEqual, nil)
			So(bucket, ShouldEqual, testURLBucket)
			So(path, ShouldEqual, testURLPath)
		})
	})
}

func TestPutS3File(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPutS3File")
	Convey("When given a file to copy to S3...", t, func() {
		Convey("a valid source file with a long key should return an error ", func() {
			//Make a test file with some random content.
			tempfile, err := ioutil.TempFile("", "randomString")
			So(err, ShouldEqual, nil)
			randStr := util.RandomString()
			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldEqual, nil)
			tempfile.Close()

			// put the test file on S3
			auth := &aws.Auth{
				AccessKey: testConfig.Providers.AWS.Id,
				SecretKey: testConfig.Providers.AWS.Secret,
			}
			longURLKey := sourceURL + strings.Repeat("suffix", 300)
			err = PutS3File(auth, tempfile.Name(), longURLKey, "application/x-tar", "public-read")
			So(err, ShouldNotEqual, nil)
		})
		Convey("a valid source file with a valid key should return no errors", func() {
			//Make a test file with some random content.
			tempfile, err := ioutil.TempFile("", "randomString")
			So(err, ShouldEqual, nil)
			randStr := util.RandomString()
			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldEqual, nil)
			tempfile.Close()

			// put the test file on S3
			auth := &aws.Auth{
				AccessKey: testConfig.Providers.AWS.Id,
				SecretKey: testConfig.Providers.AWS.Secret,
			}
			err = PutS3File(auth, tempfile.Name(), sourceURL, "application/x-tar", "public-read")
			So(err, ShouldEqual, nil)
		})
	})
}

func TestS3Copy(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestS3Copy")
	Convey("When given a source and  destination URL to copy from/to...", t, func() {
		Convey("a valid source file should be copied to the valid destination", func() {
			//Make a test file with some random content.
			tempfile, err := ioutil.TempFile("", "randomString")
			So(err, ShouldEqual, nil)
			randStr := util.RandomString()

			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldEqual, nil)
			tempfile.Close()

			// Put the test file on S3
			auth := &aws.Auth{
				AccessKey: testConfig.Providers.AWS.Id,
				SecretKey: testConfig.Providers.AWS.Secret,
			}
			err = PutS3File(auth, tempfile.Name(), sourceURL, "application/x-tar", "public-read")
			So(err, ShouldEqual, nil)

			// Copy the test file over to another location
			err = CopyS3File(auth, sourceURL, destUrl, "public-read")
			So(err, ShouldEqual, nil)

			// Ensure that the file was actually copied
			rdr, err := GetS3File(auth, destUrl)
			defer rdr.Close()
			So(err, ShouldEqual, nil)

			fileContents, err := ioutil.ReadAll(rdr)
			So(err, ShouldEqual, nil)
			So(string(fileContents), ShouldEqual, randStr)
		})
		Convey("a valid source file with a long key should return an error "+
			"even if sent to a valid destination", func() {
			//Make a test file with some random content.
			tempfile, err := ioutil.TempFile("", "randomString")
			So(err, ShouldEqual, nil)
			randStr := util.RandomString()
			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldEqual, nil)
			tempfile.Close()

			// put the test file on S3
			auth := &aws.Auth{
				AccessKey: testConfig.Providers.AWS.Id,
				SecretKey: testConfig.Providers.AWS.Secret,
			}
			longURLKey := sourceURL + strings.Repeat("suffix", 300)
			err = PutS3File(auth, tempfile.Name(), longURLKey, "application/x-tar", "public-read")
			So(err, ShouldNotEqual, nil)
		})
		Convey("a valid source file copied to a destination with too long a "+
			"key name should return an error", func() {
			//Make a test file with some random content.
			tempfile, err := ioutil.TempFile("", "randomString")
			So(err, ShouldEqual, nil)
			randStr := util.RandomString()
			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldEqual, nil)
			tempfile.Close()

			// put the test file on S3
			auth := &aws.Auth{
				AccessKey: testConfig.Providers.AWS.Id,
				SecretKey: testConfig.Providers.AWS.Secret,
			}
			err = PutS3File(auth, tempfile.Name(), sourceURL, "application/x-tar", "public-read")
			So(err, ShouldEqual, nil)

			longURLKey := sourceURL + strings.Repeat("suffix", 300)
			// Attempt to copy the test file over to another location
			err = CopyS3File(auth, sourceURL, longURLKey, "public-read")
			So(err, ShouldNotEqual, nil)

			// Ensure that the file was not copied
			rdr, err := GetS3File(auth, longURLKey)
			So(rdr, ShouldEqual, nil)
			So(err, ShouldNotEqual, nil)
		})
	})
}
