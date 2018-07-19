package thirdparty

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/stretchr/testify/assert"
)

var (
	sourceURL   = "s3://build-push-testing/test/source/testfile"
	downloadURL = "https://s3.amazonaws.com/build-push-testing/test/source/testfile"
)

func TestPutS3File(t *testing.T) {
	assert := assert.New(t)
	//Make a test file with some random content.
	tempfile, err := ioutil.TempFile("", "randomString")
	assert.NoError(err)
	randStr := util.RandomString()
	_, err = tempfile.Write([]byte(randStr))
	assert.NoError(err)
	assert.NoError(tempfile.Close())

	// put the test file on S3
	auth := &aws.Auth{
		AccessKey: testConfig.Providers.AWS.Id,
		SecretKey: testConfig.Providers.AWS.Secret,
	}
	err = PutS3File(auth, tempfile.Name(), sourceURL, "application/x-tar", "public-read")
	assert.NoError(err)

	// get s3 file and read contents
	rc, err := GetS3File(auth, sourceURL)
	assert.NoError(err)
	data, err := ioutil.ReadAll(rc)
	defer rc.Close()
	assert.NoError(err)
	assert.Equal(randStr, string(data[:]))
}

func TestPutS3FileMultiPart(t *testing.T) {
	assert := assert.New(t)

	bigBuff := make([]byte, 6000000)
	err := ioutil.WriteFile("bigfile.test0", bigBuff, 0666)
	assert.NoError(err)

	// put the test file on S3
	auth := &aws.Auth{
		AccessKey: testConfig.Providers.AWS.Id,
		SecretKey: testConfig.Providers.AWS.Secret,
	}
	err = PutS3File(auth, "bigfile.test0", sourceURL, "application/x-tar", "public-read")
	if !assert.NoError(err) {
		return
	}

	// get s3 file and read contents
	rc, err := GetS3File(auth, sourceURL)
	if !assert.NoError(err) {
		return
	}
	data, err := ioutil.ReadAll(rc)
	defer rc.Close()
	assert.NoError(err)
	assert.Equal(6000000, len(data))

	// download file directly
	resp, err := http.Get(downloadURL)
	assert.NoError(err)
	assert.Equal(http.StatusOK, resp.StatusCode)
}

func TestLegacyGetS3FileWithLegacyPut(t *testing.T) {
	assert := assert.New(t)
	//Make a test file with some random content.
	tempfile, err := ioutil.TempFile("", "randomString")
	assert.NoError(err)
	randStr := util.RandomString()
	_, err = tempfile.Write([]byte(randStr))
	assert.NoError(err)
	assert.NoError(tempfile.Close())

	// put the test file on S3 using legacy function
	auth := &aws.Auth{
		AccessKey: testConfig.Providers.AWS.Id,
		SecretKey: testConfig.Providers.AWS.Secret,
	}
	err = legacyPutS3File(auth, tempfile.Name(), sourceURL, "application/x-tar", "public-read")
	assert.NoError(err)

	// get s3 file and read contents
	rc, err := legacyGetS3File(auth, sourceURL)
	assert.NoError(err)
	data, err := ioutil.ReadAll(rc)
	defer rc.Close()
	assert.NoError(err)
	assert.Equal(randStr, string(data[:]))

	// download file directly
	resp, err := http.Get(downloadURL)
	assert.NoError(err)
	defer resp.Body.Close()
	data, err = ioutil.ReadAll(resp.Body)
	assert.NoError(err)
	assert.Equal(http.StatusOK, resp.StatusCode)
	assert.Equal(randStr, string(data[:]))
}
