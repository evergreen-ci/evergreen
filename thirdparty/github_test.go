package thirdparty

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
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
