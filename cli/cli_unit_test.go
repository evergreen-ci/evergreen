package cli

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGitCmd(t *testing.T) {

	Convey("when testing git cmd", t, func() {

		Convey("when calling out to binary with properly formatted args should"+
			"run binary", func() {
			out, err := gitCmd("--version", "")
			So(err, ShouldBeNil)

			So(out, ShouldStartWith, "git version")
		})
		Convey("when calling out to binary with poorly formatted args should"+
			"run binary and return error", func() {
			_, err := gitCmd("bad", "args")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEndWith, "failed with err exit status 1")
		})

	})

}

func TestGetHistoryCreateUrlQuery(t *testing.T) {
	Convey("with a test history parameter", t, func() {
		thp := model.TestHistoryParameters{
			Project:        "sample",
			TaskNames:      []string{"a", "b"},
			TestNames:      []string{"c", "d"},
			TestStatuses:   []string{"blah"},
			TaskStatuses:   []string{"one", "two"},
			BeforeRevision: "abc",
		}
		Convey("query parameter should have all relevant values", func() {
			So(createUrlQuery(thp), ShouldEqual,
				"testStatuses=blah&taskStatuses=one,two&tasks=a,b&tests=c,d&beforeRevision=abc")
		})
	})
}
