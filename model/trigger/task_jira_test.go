package trigger

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	projectName     = "Test Project"
	projectOwner    = "testprojowner"
	projectId       = "testproject"
	versionRevision = "aaaaaaaaaaaaaaaaaaa"
	versionMessage  = "bbbbbbbbbb"
	buildName       = "Linux 64"
	buildId         = "b1"
	taskName        = "mainTests"
	taskId          = "t1"
	testName1       = "local/jstests/big_test.js"
	testName2       = "FunUnitTest"
	testName3       = `Windows\test\cool.exe`

	hostId  = "h1"
	hostDNS = "h1.net"
)

func TestJIRASummary(t *testing.T) {
	Convey("With failed task alert types:", t, func() {
		j := jiraBuilder{
			project: "ABC",
			uiRoot:  "http://domain.invalid",
			ProjectRef: &model.ProjectRef{
				DisplayName: projectName,
				Owner:       projectOwner,
			},
			Task: &task.Task{
				DisplayName: taskName,
				Details:     apimodels.TaskEndDetail{},
			},
			Build:   &build.Build{DisplayName: buildName},
			Version: &version.Version{Revision: versionRevision},
		}

		Convey("a task that timed out should return a subject", func() {
			j.Task.Details.TimedOut = true
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting the time out and showing the task name", func() {
				So(subj, ShouldContainSubstring, "Timed Out")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldNotContainSubstring, versionRevision[0:9])
				So(subj, ShouldContainSubstring, projectName)
			})
		})
		Convey("a task that failed on a system command should return a subject", func() {
			j.Task.Details.Type = model.SystemCommandType
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting the system failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "System")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
			})
		})
		Convey("a task that failed on a normal command with no tests should return a subject", func() {
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "Failed")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
			})
		})
		Convey("a task with two failed tests should return a subject", func() {
			j.Task.LocalTestResults = []task.TestResult{
				{TestFile: testName1, Status: evergreen.TestFailedStatus},
				{TestFile: testName2, Status: evergreen.TestFailedStatus},
				{TestFile: testName3, Status: evergreen.TestSucceededStatus},
				{TestFile: testName3, Status: evergreen.TestSucceededStatus},
				{TestFile: testName3, Status: evergreen.TestSucceededStatus},
				{TestFile: testName3, Status: evergreen.TestSucceededStatus},
			}
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name and failed tests", func() {
				So(subj, ShouldContainSubstring, "Failures")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
				So(subj, ShouldContainSubstring, "big_test.js")
				So(subj, ShouldContainSubstring, "FunUnitTest")
				So(subj, ShouldNotContainSubstring, "cool.exe")
				Convey("with test names properly truncated", func() {
					So(subj, ShouldNotContainSubstring, "local")
					So(subj, ShouldNotContainSubstring, "jstest")
				})
			})
		})
		Convey("a task with failing tests should return a subject omitting any silently failing tests", func() {
			j.Task.LocalTestResults = []task.TestResult{
				{TestFile: testName1, Status: evergreen.TestFailedStatus},
				{TestFile: testName2, Status: evergreen.TestFailedStatus},
				{TestFile: testName3, Status: evergreen.TestSilentlyFailedStatus},
			}
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name and failed tests", func() {
				So(subj, ShouldContainSubstring, "Failures")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
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
			reallyLongTestName := ""
			for i := 0; i < 300; i++ {
				reallyLongTestName = reallyLongTestName + "a"
			}
			j.Task.LocalTestResults = []task.TestResult{
				{TestFile: testName1, Status: evergreen.TestFailedStatus},
				{TestFile: testName2, Status: evergreen.TestFailedStatus},
				{TestFile: testName3, Status: evergreen.TestFailedStatus},
				{TestFile: testName3, Status: evergreen.TestFailedStatus},
				{TestFile: testName3, Status: evergreen.TestFailedStatus},
				{TestFile: reallyLongTestName, Status: evergreen.TestFailedStatus},
			}
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("and list the tests, but not exceed 254 characters", func() {
				So(subj, ShouldContainSubstring, "Failures")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
				So(subj, ShouldContainSubstring, "big_test.js")
				So(subj, ShouldContainSubstring, "FunUnitTest")
				So(subj, ShouldContainSubstring, "cool.exe")
				So(subj, ShouldContainSubstring, "+1 more")
				So(len(subj), ShouldBeLessThanOrEqualTo, 255)
			})
		})
		Convey("a failed task with passing tests should return a subject", func() {
			j.Task.LocalTestResults = []task.TestResult{
				{TestFile: testName1, Status: evergreen.TestSucceededStatus},
				{TestFile: testName2, Status: evergreen.TestSucceededStatus},
				{TestFile: testName3, Status: evergreen.TestSucceededStatus},
			}
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting a task failure without a parenthetical", func() {
				So(subj, ShouldContainSubstring, "Failed")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
				So(subj, ShouldNotContainSubstring, "big_test.js")
				So(subj, ShouldNotContainSubstring, "FunUnitTest")
				So(subj, ShouldNotContainSubstring, "cool.exe")
				So(subj, ShouldNotContainSubstring, "(")
				So(subj, ShouldNotContainSubstring, ")")
			})
		})
		Convey("a failed task with only passing or silently failing tests should return a subject", func() {
			j.Task.LocalTestResults = []task.TestResult{
				{TestFile: testName1, Status: evergreen.TestSilentlyFailedStatus},
				{TestFile: testName2, Status: evergreen.TestSucceededStatus},
				{TestFile: testName3, Status: evergreen.TestSilentlyFailedStatus},
			}
			subj := j.getSummary()
			So(subj, ShouldNotEqual, "")
			Convey("denoting a task failure without a parenthetical", func() {
				So(subj, ShouldContainSubstring, "Failed")
				So(subj, ShouldContainSubstring, taskName)
				So(subj, ShouldContainSubstring, buildName)
				So(subj, ShouldContainSubstring, versionRevision[0:8])
				So(subj, ShouldContainSubstring, projectName)
				So(subj, ShouldNotContainSubstring, "big_test.js")
				So(subj, ShouldNotContainSubstring, "FunUnitTest")
				So(subj, ShouldNotContainSubstring, "cool.exe")
				So(subj, ShouldNotContainSubstring, "(")
				So(subj, ShouldNotContainSubstring, ")")
			})
		})
	})
}

func TestJIRADescription(t *testing.T) {
	Convey("With a failed task context", t, func() {
		j := jiraBuilder{
			uiRoot: "http://evergreen.ui",
			ProjectRef: &model.ProjectRef{
				DisplayName: projectName,
				Identifier:  projectId,
				Owner:       projectOwner,
			},
			Task: &task.Task{
				Id:          taskId,
				DisplayName: taskName,
				Details:     apimodels.TaskEndDetail{},
				Project:     projectId,
				LocalTestResults: []task.TestResult{
					{TestFile: testName1, Status: evergreen.TestFailedStatus, URL: "direct_link"},
					{TestFile: testName2, Status: evergreen.TestFailedStatus, LogId: "123"},
					{TestFile: testName3, Status: evergreen.TestSucceededStatus},
				},
			},
			Host:  &host.Host{Id: hostId, Host: hostDNS},
			Build: &build.Build{DisplayName: buildName, Id: buildId},
			Version: &version.Version{
				Revision: versionRevision,
				Message:  versionMessage,
			},
		}
		Convey("the description should be successfully generated", func() {
			d, err := j.getDescription()
			So(err, ShouldBeNil)
			So(d, ShouldNotEqual, "")

			Convey("the task, host, project, and build names should be present", func() {
				So(d, ShouldContainSubstring, taskName)
				So(d, ShouldContainSubstring, hostDNS)
				So(d, ShouldContainSubstring, projectName)
				So(d, ShouldContainSubstring, buildName)
				So(d, ShouldContainSubstring, projectOwner)
				So(d, ShouldContainSubstring, versionRevision)
				So(d, ShouldContainSubstring, versionMessage)
				So(d, ShouldContainSubstring, "diff|https://github.com/")
			})
			Convey("with links to the task, host, project", func() {
				So(d, ShouldContainSubstring, taskId)
				So(d, ShouldContainSubstring, hostId)
				So(d, ShouldContainSubstring, projectId)
			})
			Convey("and the failed tasks should be listed with links", func() {
				So(d, ShouldContainSubstring, cleanTestName(testName1))
				So(d, ShouldContainSubstring, "direct_link")
				So(d, ShouldContainSubstring, cleanTestName(testName2))
				So(d, ShouldContainSubstring, "test_log/123")
				Convey("but passing tasks should not be present", func() {
					So(d, ShouldNotContainSubstring, cleanTestName(testName3))
				})
			})
		})
	})
}
