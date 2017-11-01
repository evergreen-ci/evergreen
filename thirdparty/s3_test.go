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
	sourceURL = "s3://build-push-testing/test/source/testfile"
)

func TestPutS3File(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPutS3File")
	Convey("When given a file to copy to S3...", t, func() {
		Convey("a valid source file with a long key should return an error ", func() {
			//Make a test file with some random content.
			tempfile, err := ioutil.TempFile("", "randomString")
			So(err, ShouldBeNil)
			randStr := util.RandomString()
			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldBeNil)
			So(tempfile.Close(), ShouldBeNil)

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
			So(err, ShouldBeNil)
			randStr := util.RandomString()
			_, err = tempfile.Write([]byte(randStr))
			So(err, ShouldBeNil)
			So(tempfile.Close(), ShouldBeNil)

			// put the test file on S3
			auth := &aws.Auth{
				AccessKey: testConfig.Providers.AWS.Id,
				SecretKey: testConfig.Providers.AWS.Secret,
			}
			err = PutS3File(auth, tempfile.Name(), sourceURL, "application/x-tar", "public-read")
			So(err, ShouldBeNil)
		})
	})
}
