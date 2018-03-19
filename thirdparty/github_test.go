package thirdparty

import (
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

func TestCheckGithubAPILimit(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCheckGithubAPILimit")
	Convey("Getting Github API limit should succeed", t, func() {
		rem, err := CheckGithubAPILimit(testConfig.Credentials[repoKind])
		// Without a proper token, the API limit is small
		So(rem, ShouldBeGreaterThan, 100)
		So(err, ShouldBeNil)
	})
}
