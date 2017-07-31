package alerts

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/render"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	ProjectName     = "Test Project"
	ProjectOwner    = "testprojowner"
	ProjectId       = "testproject"
	VersionRevision = "aaaaaaaaaaaaaaaaaaa"
	VersionMessage  = "bbbbbbbbbb"
	BuildName       = "Linux 64"
	BuildId         = "b1"
	TaskName        = "mainTests"
	TaskId          = "t1"
	TestName1       = "local/jstests/big_test.js"
	TestName2       = "FunUnitTest"
	TestName3       = `Windows\test\cool.exe`
)

func TestEmailSubject(t *testing.T) {
	Convey("With failed task alert types:", t, func() {
		ctx := AlertContext{
			AlertRequest: &alert.AlertRequest{
				Trigger: alertrecord.TaskFailedId,
			},
			ProjectRef: &model.ProjectRef{DisplayName: ProjectName},
			Task: &task.Task{
				DisplayName: TaskName,
				Details:     apimodels.TaskEndDetail{},
			},
			Build:   &build.Build{DisplayName: BuildName},
			Version: &version.Version{Revision: VersionRevision},
		}
		Convey("a task that timed out should return a subject", func() {
			ctx.Task.Details.TimedOut = true
			subj := getSubject(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the time out and showing the task name", func() {
				So(subj, ShouldContainSubstring, "Timed Out")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldNotContainSubstring, VersionRevision[0:9])
				So(subj, ShouldContainSubstring, ProjectName)
			})
		})
		Convey("a task that failed on a system command should return a subject", func() {
			ctx.Task.Details.Type = model.SystemCommandType
			subj := getSubject(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the system failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "System")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldContainSubstring, ProjectName)
			})
		})
		Convey("a task that has a hearbeat failure should return a subject", func() {
			ctx.Task.Details.Description = task.AgentHeartbeat
			subj := getSummary(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the system failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "System Failure")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldNotContainSubstring, VersionRevision[0:9])
				So(subj, ShouldContainSubstring, ProjectName)
			})
		})
		Convey("a task that failed on a normal command with no tests should return a subject", func() {
			subj := getSubject(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "Task Failed")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldContainSubstring, ProjectName)
			})
		})
		Convey("a task with two failed tests should return a subject", func() {
			ctx.Task.TestResults = []task.TestResult{
				{TestFile: TestName1, Status: evergreen.TestFailedStatus},
				{TestFile: TestName2, Status: evergreen.TestFailedStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
			}
			subj := getSubject(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name and failed tests", func() {
				So(subj, ShouldContainSubstring, "Test Failures")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldContainSubstring, ProjectName)
				So(subj, ShouldContainSubstring, "big_test.js")
				So(subj, ShouldContainSubstring, "FunUnitTest")
				So(subj, ShouldNotContainSubstring, "cool.exe")
				Convey("with test names properly truncated", func() {
					So(subj, ShouldNotContainSubstring, "local")
					So(subj, ShouldNotContainSubstring, "jstest")
				})
			})
		})
		Convey("a task with five failed tests should return a subject", func() {
			ctx.Task.TestResults = []task.TestResult{
				{TestFile: TestName1, Status: evergreen.TestFailedStatus},
				{TestFile: TestName2, Status: evergreen.TestFailedStatus},
				{TestFile: TestName3, Status: evergreen.TestFailedStatus},
				{TestFile: TestName3, Status: evergreen.TestFailedStatus},
				{TestFile: TestName3, Status: evergreen.TestFailedStatus},
			}
			subj := getSubject(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting two test failures but hiding the rest", func() {
				So(subj, ShouldContainSubstring, "Test Failures")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldContainSubstring, ProjectName)
				So(subj, ShouldContainSubstring, "big_test.js")
				So(subj, ShouldContainSubstring, "FunUnitTest")
				So(subj, ShouldNotContainSubstring, "cool.exe")
				So(subj, ShouldContainSubstring, "+3 more")
			})
		})
		Convey("a failed task with passing tests should return a subject", func() {
			ctx.Task.TestResults = []task.TestResult{
				{TestFile: TestName1, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName2, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
			}
			subj := getSubject(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting a task failure without a parenthetical", func() {
				So(subj, ShouldContainSubstring, "Task Failed")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldContainSubstring, ProjectName)
				So(subj, ShouldNotContainSubstring, "big_test.js")
				So(subj, ShouldNotContainSubstring, "FunUnitTest")
				So(subj, ShouldNotContainSubstring, "cool.exe")
				So(subj, ShouldNotContainSubstring, "(")
				So(subj, ShouldNotContainSubstring, ")")
			})
		})

	})

}

func TestEmailBody(t *testing.T) {
	Convey("With failed task alert types:", t, func() {
		ctx := AlertContext{
			AlertRequest: &alert.AlertRequest{
				Trigger: alertrecord.TaskFailedId,
			},
			ProjectRef: &model.ProjectRef{DisplayName: ProjectName},
			Task: &task.Task{
				DisplayName: TaskName,
				Details:     apimodels.TaskEndDetail{},
				Id:          "t123",
			},
			Build:    &build.Build{DisplayName: BuildName},
			Version:  &version.Version{Revision: VersionRevision},
			Settings: testutil.TestConfig(),
		}
		Convey("a task with two failed tests should return a body with valid links", func() {
			ctx.Task.TestResults = []task.TestResult{
				{TestFile: TestName1, Status: evergreen.TestFailedStatus},
				{TestFile: TestName2, Status: evergreen.TestFailedStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
			}
			ctx.FailedTests = []task.TestResult{
				{URL: "test_log/321", TestFile: TestName1, Status: evergreen.TestFailedStatus},
				{URL: "test_log/666", TestFile: TestName2, Status: evergreen.TestFailedStatus},
			}
			home := evergreen.FindEvergreenHome()

			ed := &EmailDeliverer{
				SMTPSettings{
					From:     "",
					Server:   "",
					Port:     999,
					Username: "",
					Password: "",
					UseSSL:   false,
				},
				render.New(render.Options{
					Directory: filepath.Join(home, "alerts", "templates"),
				}),
			}
			body, err := ed.getBody(ctx)
			if err != nil {
				t.Error(err)
			}
			So(body, ShouldNotBeEmpty)
			So(body, ShouldContainSubstring, "href=\"localhost/task/"+ctx.Task.Id+"\"")
			So(body, ShouldContainSubstring, "href=\"localhost/"+ctx.FailedTests[0].URL+"\"")
		})
	})
}
