package thirdparty

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/testutils"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var testConfig = mci.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
}

func TestGetGithubCommits(t *testing.T) {
	testutils.ConfigureIntegrationTest(t, testConfig, "TestGetGithubCommits")
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
