package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type AdminDataSuite struct {
	ctx Connector
	suite.Suite
}

func TestDataConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &DBConnector{}
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.HandleTestingErr(db.ClearCollections(admin.Collection, task.Collection, task.OldCollection, build.Collection, version.Collection), t,
		"Error clearing collections")
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &version.Version{
		Id:     b.Version,
		Status: evergreen.VersionStarted,
	}
	testTask1 := &task.Task{
		Id:        "taskToRestart",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
	}
	testTask2 := &task.Task{
		Id:        "taskThatSucceeded",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskSucceeded,
	}
	testTask3 := &task.Task{
		Id:        "taskOutsideOfTimeRange",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 11, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
	}
	p := &model.ProjectRef{
		Identifier: "sample",
	}

	b.Tasks = []build.TaskCache{
		{
			Id: testTask1.Id,
		},
		{
			Id: testTask2.Id,
		},
		{
			Id: testTask3.Id,
		},
	}
	testutil.HandleTestingErr(b.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(v.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(testTask1.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(testTask2.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(testTask3.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(p.Insert(), t, "error inserting documents")
	suite.Run(t, s)
}

func TestMockConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &MockConnector{}
	suite.Run(t, s)
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	u := &user.DBUser{Id: "user"}
	settings := &admin.AdminSettings{
		Banner:      "test banner",
		BannerTheme: admin.Warning,
		ServiceFlags: admin.ServiceFlags{
			NotificationsDisabled: true,
			TaskrunnerDisabled:    true,
		},
	}

	err := s.ctx.SetAdminSettings(settings, u)
	s.NoError(err)

	settingsFromConnector, err := s.ctx.GetAdminSettings()
	s.NoError(err)
	s.Equal(settings.Banner, settingsFromConnector.Banner)
	s.Equal(settings.ServiceFlags, settingsFromConnector.ServiceFlags)
	s.Equal(admin.Warning, string(settings.BannerTheme))
}

func (s *AdminDataSuite) TestRestart() {
	startTime := time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	endTime := time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local)
	userName := "user"

	// test dry run
	dryRunResp, err := s.ctx.RestartFailedTasks(startTime, endTime, userName, model.RestartTaskOptions{
		DryRun:     true,
		OnlyRed:    false,
		OnlyPurple: false})
	s.NoError(err)
	s.NotZero(len(dryRunResp.TasksRestarted))
	s.Nil(dryRunResp.TasksErrored)

	// test restarting tasks
	realResp, err := s.ctx.RestartFailedTasks(startTime, endTime, userName, model.RestartTaskOptions{
		DryRun:     false,
		OnlyRed:    false,
		OnlyPurple: false})
	s.NoError(err)
	s.NotZero(len(realResp.TasksRestarted))
	s.Nil(realResp.TasksErrored)
	s.Equal(len(dryRunResp.TasksRestarted), len(realResp.TasksRestarted))
}
