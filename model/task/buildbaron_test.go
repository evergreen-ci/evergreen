package task

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/model/testresult"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTaskToJQL(t *testing.T) {
	Convey("Given a task with with two failed tests and one successful test, "+
		"the jql should contain only the failed test names", t, func() {
		task1 := &Task{}
		task1.LocalTestResults = []testresult.TestResult{
			{Status: "fail", TestName: "foo.js"},
			{Status: "success", TestName: "bar.js"},
			{Status: "fail", TestName: "baz.js"},
		}
		task1.DisplayName = "foobar"
		jQL1 := task1.GetJQL([]string{"PRJ"})
		referenceJQL1 := fmt.Sprintf(jqlBFQuery, "PRJ", "text~\"foo.js\" or text~\"baz.js\"")
		So(jQL1, ShouldEqual, referenceJQL1)
	})

	Convey("Given a task with with oo failed tests, "+
		"the jql should contain only the failed task name", t, func() {
		task2 := &Task{}
		task2.LocalTestResults = []testresult.TestResult{}
		task2.DisplayName = "foobar"
		jQL2 := task2.GetJQL([]string{"PRJ"})
		referenceJQL2 := fmt.Sprintf(jqlBFQuery, "PRJ", "text~\"foobar\"")
		So(jQL2, ShouldEqual, referenceJQL2)
	})
}
