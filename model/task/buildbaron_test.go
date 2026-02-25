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
		tsk := &Task{}
		tsk.LocalTestResults = []testresult.TestResult{
			{Status: "fail", TestName: "hash_abc123", DisplayTestName: "foo.js"},
			{Status: "success", TestName: "hash_def456", DisplayTestName: "bar.js"},
			{Status: "fail", TestName: "hash_ghi789", DisplayTestName: "baz.js"},
		}
		tsk.DisplayName = "foobar"
		jql := tsk.GetJQL([]string{"PRJ"})
		expectedJQL := fmt.Sprintf(jqlBFQuery, "PRJ", `text~"foo.js" or text~"baz.js"`)
		So(jql, ShouldEqual, expectedJQL)
	})

	Convey("Given a task with with no failed tests, "+
		"the jql should contain only the failed task name", t, func() {
		tsk := &Task{}
		tsk.LocalTestResults = []testresult.TestResult{}
		tsk.DisplayName = "foobar"
		jql := tsk.GetJQL([]string{"PRJ"})
		expectedJQL := fmt.Sprintf(jqlBFQuery, "PRJ", "text~\"foobar\"")
		So(jql, ShouldEqual, expectedJQL)
	})

	Convey("Given a task with failed test containing path separators, "+
		"the jql should contain only the filename", t, func() {
		tsk := &Task{}
		tsk.LocalTestResults = []testresult.TestResult{
			{Status: "fail", TestName: "hash_xyz", DisplayTestName: "path/to/GSSAPI_Auth.Test.js"},
		}
		tsk.DisplayName = "test_task"
		jql := tsk.GetJQL([]string{"PRJ"})
		expectedJQL := fmt.Sprintf(jqlBFQuery, "PRJ", `text~"GSSAPI_Auth.Test.js"`)
		So(jql, ShouldEqual, expectedJQL)
	})

	Convey("Given a task with failed test containing special JQL characters, "+
		"the jql should properly escape them", t, func() {
		tsk := &Task{}
		tsk.LocalTestResults = []testresult.TestResult{
			{Status: "fail", TestName: "hash_special", DisplayTestName: "Test-Name_With.Special-Chars"},
		}
		tsk.DisplayName = "test_task"
		jql := tsk.GetJQL([]string{"PRJ"})
		expectedJQL := fmt.Sprintf(jqlBFQuery, "PRJ", `text~"Test\\-Name_With.Special\\-Chars"`)
		So(jql, ShouldEqual, expectedJQL)
	})

	Convey("Given a task with failed test with empty display test name, "+
		"the jql should fallback to its test name", t, func() {
		tsk := &Task{}
		tsk.LocalTestResults = []testresult.TestResult{
			{Status: "fail", TestName: "legacy_test.js", DisplayTestName: ""},
		}
		tsk.DisplayName = "test_task"
		jql := tsk.GetJQL([]string{"PRJ"})
		expectedJQL := fmt.Sprintf(jqlBFQuery, "PRJ", `text~"legacy_test.js"`)
		So(jql, ShouldEqual, expectedJQL)
	})
}
