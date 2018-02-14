package thirdparty

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

var repoKind = "github"

func TestGetGithubAPIStatus(t *testing.T) {
	Convey("Getting Github API status should yield expected response", t, func() {
		status, err := GetGithubAPIStatus()
		So(err, ShouldBeNil)
		So(status, ShouldBeIn, []string{GithubAPIStatusGood, GithubAPIStatusMinor, GithubAPIStatusMajor})
	})
}

func TestVerifyGithubAPILimitHeader(t *testing.T) {
	Convey("With an http.Header", t, func() {
		header := http.Header{}
		Convey("An empty limit header should return an error", func() {
			header["X-Ratelimit-Remaining"] = []string{"5000"}
			rem, err := verifyGithubAPILimitHeader(header)
			So(err, ShouldNotBeNil)
			So(rem, ShouldEqual, 0)
		})

		Convey("An empty remaining header should return an error", func() {
			header["X-Ratelimit-Limit"] = []string{"5000"}
			delete(header, "X-Ratelimit-Remaining")
			rem, err := verifyGithubAPILimitHeader(header)
			So(err, ShouldNotBeNil)
			So(rem, ShouldEqual, 0)
		})

		Convey("It should return remaining", func() {
			header["X-Ratelimit-Limit"] = []string{"5000"}
			header["X-Ratelimit-Remaining"] = []string{"4000"}
			rem, err := verifyGithubAPILimitHeader(header)
			So(err, ShouldBeNil)
			So(rem, ShouldEqual, 4000)
		})
	})
}

func TestCheckGithubAPILimit(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCheckGithubAPILimit")
	Convey("Getting Github API limit should succeed", t, func() {
		rem, err := CheckGithubAPILimit(testConfig.Credentials[repoKind])
		// Without a proper token, the API limit is small
		So(rem, ShouldBeGreaterThan, 100)
		So(err, ShouldBeNil)
	})
}

func TestUnmarshalGithubTime(t *testing.T) {
	assert := assert.New(t)

	var gt GithubTime
	timeStr := `"2018-02-07T19:54:26Z"`
	assert.Zero(gt)
	err := gt.UnmarshalJSON([]byte(timeStr))
	assert.NoError(err)
	assert.NotZero(gt)
	assert.Equal("2018-02-07 19:54:26 +0000 UTC", gt.Time().String())

	gt = GithubTime{}
	assert.Zero(gt)
	timeStr = `"2013-05-17T15:42:08Z"`
	err = gt.UnmarshalJSON([]byte(timeStr))
	assert.NoError(err)
	assert.NotZero(gt)
	assert.Equal("2013-05-17 15:42:08 +0000 UTC", gt.Time().String())

	gt = GithubTime{}
	timeStr = `2018-02-07T19:54:26Z`
	err = gt.UnmarshalJSON([]byte(timeStr))
	assert.Error(err)
	assert.Equal("expected JSON string while parsing time: invalid character '-' after top-level value", err.Error())
	assert.Zero(gt)

	gt = GithubTime{}
	timeStr = `"2018-0"`
	err = gt.UnmarshalJSON([]byte(timeStr))
	assert.Error(err)
	assert.Equal("Error parsing time '2018-0' in UTC time zone: parsing time \"2018-0\": month out of range", err.Error())
	assert.Zero(gt)

	gt = GithubTime{}
	timeStr = `""`
	err = gt.UnmarshalJSON([]byte(timeStr))
	assert.Error(err)
	assert.Equal("expected JSON string while parsing time", err.Error())
	assert.Zero(gt)

	gt = GithubTime{}
	timeStr = `null`
	err = gt.UnmarshalJSON([]byte(timeStr))
	assert.Error(err)
	assert.Equal("expected JSON string while parsing time", err.Error())
	assert.Zero(gt)
}

func TestMarshalGithubTime(t *testing.T) {
	assert := assert.New(t)

	var gt GithubTime
	timeStr := `"2018-02-07T19:54:26Z"`
	assert.Zero(gt)
	err := gt.UnmarshalJSON([]byte(timeStr))
	assert.NoError(err)
	assert.NotZero(gt)
	assert.Equal("2018-02-07 19:54:26 +0000 UTC", gt.Time().String())

	b, err := gt.MarshalJSON()
	assert.NoError(err)
	assert.Equal(timeStr, string(b))
}
