package thirdparty

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetGithubAPIStatus(t *testing.T) {
	Convey("Getting Github API status should yield expected response", t, func() {
		status, err := GetGithubAPIStatus()
		So(err, ShouldBeNil)
		So(status, ShouldBeIn, []string{GithubAPIStatusGood, GithubAPIStatusMinor, GithubAPIStatusMajor})
	})
}
