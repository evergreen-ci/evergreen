package thirdparty

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

var testConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func TestGetGithubCommits(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetGithubCommits")
	Convey("When requesting github commits with a valid OAuth token...", t, func() {
		Convey("Fetching commits from the repository should not return any errors", func() {
			commitsURL := "https://api.github.com/repos/deafgoat/mci-test/commits"
			_, _, err := GetGithubCommits(testConfig.Credentials["github"], commitsURL)
			So(err, ShouldBeNil)
		})

		Convey("Fetching commits from the repository should return all available commits", func() {
			commitsURL := "https://api.github.com/repos/deafgoat/mci-test/commits"
			githubCommits, _, err := GetGithubCommits(testConfig.Credentials["github"], commitsURL)
			So(err, ShouldBeNil)
			So(len(githubCommits), ShouldEqual, 3)
		})
	})
}
