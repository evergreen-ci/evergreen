package buildbaron

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	reporting.QuietMode()
}

func TestTaskToJQL(t *testing.T) {
	Convey("Given a task with with two failed tests and one successful test, "+
		"the jql should contain only the failed test names", t, func() {
		task1 := task.Task{}
		task1.LocalTestResults = []task.TestResult{
			{Status: "fail", TestFile: "foo.js"},
			{Status: "success", TestFile: "bar.js"},
			{Status: "fail", TestFile: "baz.js"},
		}
		task1.DisplayName = "foobar"
		jQL1 := taskToJQL(&task1, []string{"PRJ"})
		referenceJQL1 := fmt.Sprintf(JQLBFQuery, "PRJ", "text~\"foo.js\" or text~\"baz.js\"")
		So(jQL1, ShouldEqual, referenceJQL1)
	})

	Convey("Given a task with with oo failed tests, "+
		"the jql should contain only the failed task name", t, func() {
		task2 := task.Task{}
		task2.LocalTestResults = []task.TestResult{}
		task2.DisplayName = "foobar"
		jQL2 := taskToJQL(&task2, []string{"PRJ"})
		referenceJQL2 := fmt.Sprintf(JQLBFQuery, "PRJ", "text~\"foobar\"")
		So(jQL2, ShouldEqual, referenceJQL2)
	})
}
