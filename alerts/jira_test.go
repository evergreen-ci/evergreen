package alerts

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestJIRASummary(t *testing.T) {
	Convey("With failed task alert types:", t, func() {
		ctx := AlertContext{
			AlertRequest: &alert.AlertRequest{
				Trigger: alertrecord.TaskFailedId,
			},
			ProjectRef: &model.ProjectRef{
				DisplayName: ProjectName,
				Owner:       ProjectOwner,
			},
			Task: &task.Task{
				DisplayName: TaskName,
				Details:     apimodels.TaskEndDetail{},
			},
			Build:   &build.Build{DisplayName: BuildName},
			Version: &version.Version{Revision: VersionRevision},
		}

		Convey("a task that timed out should return a subject", func() {
			ctx.Task.Details.TimedOut = true
			subj := getSummary(ctx)
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
			subj := getSummary(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the system failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "System")
				So(subj, ShouldContainSubstring, TaskName)
				So(subj, ShouldContainSubstring, BuildName)
				So(subj, ShouldContainSubstring, VersionRevision[0:8])
				So(subj, ShouldContainSubstring, ProjectName)
			})
		})
		Convey("a task that failed on a normal command with no tests should return a subject", func() {
			subj := getSummary(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name", func() {
				So(subj, ShouldContainSubstring, "Failed")
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
			subj := getSummary(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting the failure and showing the task name and failed tests", func() {
				So(subj, ShouldContainSubstring, "Failures")
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
			subj := getSummary(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting two test failures but hiding the rest", func() {
				So(subj, ShouldContainSubstring, "Failures")
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
			subj := getSummary(ctx)
			So(subj, ShouldNotEqual, "")
			Convey("denoting a task failure without a parenthetical", func() {
				So(subj, ShouldContainSubstring, "Failed")
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

const (
	HostId  = "h1"
	HostDNS = "h1.net"
)

func TestJIRADescription(t *testing.T) {
	Convey("With a failed task context", t, func() {
		ui := "http://evergreen.ui"
		ctx := AlertContext{
			AlertRequest: &alert.AlertRequest{
				Trigger: alertrecord.TaskFailedId,
			},
			ProjectRef: &model.ProjectRef{
				DisplayName: ProjectName,
				Identifier:  ProjectId,
				Owner:       ProjectOwner,
			},
			Task: &task.Task{
				Id:          TaskId,
				DisplayName: TaskName,
				Details:     apimodels.TaskEndDetail{},
				Project:     ProjectId,
				TestResults: []task.TestResult{
					{TestFile: TestName1, Status: evergreen.TestFailedStatus, URL: "direct_link"},
					{TestFile: TestName2, Status: evergreen.TestFailedStatus, LogId: "123"},
					{TestFile: TestName3, Status: evergreen.TestSucceededStatus},
				},
			},
			Host:  &host.Host{Id: HostId, Host: HostDNS},
			Build: &build.Build{DisplayName: BuildName, Id: BuildId},
			Version: &version.Version{
				Revision: VersionRevision,
				Message:  VersionMessage,
			},
		}
		Convey("the description should be successfully generated", func() {
			d, err := getDescription(ctx, ui)
			So(err, ShouldBeNil)
			So(d, ShouldNotEqual, "")

			Convey("the task, host, project, and build names should be present", func() {
				So(d, ShouldContainSubstring, TaskName)
				So(d, ShouldContainSubstring, HostDNS)
				So(d, ShouldContainSubstring, ProjectName)
				So(d, ShouldContainSubstring, BuildName)
				So(d, ShouldContainSubstring, ProjectOwner)
				So(d, ShouldContainSubstring, VersionRevision)
				So(d, ShouldContainSubstring, VersionMessage)
				So(d, ShouldContainSubstring, "diff|https://github.com/")
			})
			Convey("with links to the task, host, project", func() {
				So(d, ShouldContainSubstring, TaskId)
				So(d, ShouldContainSubstring, HostId)
				So(d, ShouldContainSubstring, ProjectId)
			})
			Convey("and the failed tasks should be listed with links", func() {
				So(d, ShouldContainSubstring, cleanTestName(TestName1))
				So(d, ShouldContainSubstring, "direct_link")
				So(d, ShouldContainSubstring, cleanTestName(TestName2))
				So(d, ShouldContainSubstring, "test_log/123")
				Convey("but passing tasks should not be present", func() {
					So(d, ShouldNotContainSubstring, cleanTestName(TestName3))
				})
			})
		})
	})
}

type mockJIRAHandler struct {
	err     error
	proj    string
	desc    string
	summary string
}

func (mj *mockJIRAHandler) CreateTicket(fields map[string]interface{}) (
	*thirdparty.JiraCreateTicketResponse, error) {
	if mj.err != nil {
		return nil, mj.err
	}
	// if these lookups panic then that's fine -- the test will fail
	mj.proj = fields["project"].(map[string]string)["key"]
	mj.summary = fields["summary"].(string)
	mj.desc = fields["description"].(string)
	return &thirdparty.JiraCreateTicketResponse{Key: mj.proj + "-1"}, nil
}

func (mj *mockJIRAHandler) JiraHost() string { return "mock" }

func TestJIRADelivery(t *testing.T) {
	Convey("With a failed task context and alertConf", t, func() {
		ui := "http://evergreen.ui"
		ctx := AlertContext{
			AlertRequest: &alert.AlertRequest{
				Trigger: alertrecord.TaskFailedId,
			},
			ProjectRef: &model.ProjectRef{DisplayName: ProjectName, Identifier: ProjectId},
			Task: &task.Task{
				Id:          TaskId,
				DisplayName: TaskName,
				Details:     apimodels.TaskEndDetail{},
				Project:     ProjectId,
			},
			Host:    &host.Host{Id: HostId, Host: HostDNS},
			Build:   &build.Build{DisplayName: BuildName, Id: BuildId},
			Version: &version.Version{Revision: VersionRevision},
		}
		alertConf := model.AlertConfig{Provider: JiraProvider}
		Convey("and a JIRA Deliverer with a valid mock jiraCreator", func() {
			mjc := &mockJIRAHandler{}
			jd := &jiraDeliverer{
				project: "COOL",
				handler: mjc,
				uiRoot:  ui,
			}
			Convey("a populated ticket should be created successfully", func() {
				So(jd.Deliver(ctx, alertConf), ShouldBeNil)
				So(mjc.desc, ShouldNotEqual, "")
				So(mjc.summary, ShouldNotEqual, "")
				So(mjc.proj, ShouldEqual, "COOL")
			})
		})
		Convey("and a JIRA Deliverer with an erroring jiraCreator", func() {
			mjc := &mockJIRAHandler{err: errors.New("bad internet uh-oh")}
			jd := &jiraDeliverer{
				project: "COOL",
				handler: mjc,
				uiRoot:  ui,
			}
			Convey("should return an error", func() {
				So(jd.Deliver(ctx, alertConf), ShouldNotBeNil)
			})
		})
	})
}

func TestGetJIRADeliverer(t *testing.T) {
	Convey("With a JIRA alertConf and QueueProcessor", t, func() {
		alertConf := model.AlertConfig{
			Provider: JiraProvider,
			Settings: map[string]interface{}{"project": "TEST", "issue": "Bug"},
		}
		qp := &QueueProcessor{}
		Convey("a QueueProcessor with full settings should return the JIRA deliverer", func() {
			qp.config = &evergreen.Settings{
				Jira: evergreen.JiraConfig{
					Username: "u1",
					Password: "pw",
					Host:     "www.example.com",
				},
				Ui: evergreen.UIConfig{
					Url: "root",
				},
			}
			d, err := qp.getDeliverer(alertConf)
			So(err, ShouldBeNil)
			So(d, ShouldNotBeNil)
			jd, ok := d.(*jiraDeliverer)
			So(ok, ShouldBeTrue)
			So(jd.handler, ShouldNotBeNil)
			So(jd.project, ShouldEqual, "TEST")
			So(jd.uiRoot, ShouldEqual, "root")
		})

		Convey("a QueueProcessor with a malformed alertConf should error", func() {
			alertConf.Settings = map[string]interface{}{"NOTproject": "TEST"}
			d, err := qp.getDeliverer(alertConf)
			So(err, ShouldNotBeNil)
			So(d, ShouldBeNil)
			alertConf.Settings = map[string]interface{}{"project": 1000}
			d, err = qp.getDeliverer(alertConf)
			So(err, ShouldNotBeNil)
			So(d, ShouldBeNil)
		})

		Convey("a QueueProcessor with missing JIRA settings should error", func() {
			qp.config = &evergreen.Settings{
				Ui: evergreen.UIConfig{
					Url: "root",
				},
			}
			d, err := qp.getDeliverer(alertConf)
			So(err, ShouldNotBeNil)
			So(d, ShouldBeNil)
		})
		Convey("a QueueProcessor with missing UI settings should error", func() {
			qp.config = &evergreen.Settings{
				Jira: evergreen.JiraConfig{
					Username: "u1",
					Password: "pw",
					Host:     "www.example.com",
				},
			}
			d, err := qp.getDeliverer(alertConf)
			So(err, ShouldNotBeNil)
			So(d, ShouldBeNil)
		})
	})
}
