package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type hostMonitorSuite struct {
	suite.Suite
	env    evergreen.Environment
	sender *send.InternalSender
	cloud  cloud.MockProvider
}

func TestHostMonitoringSuite(t *testing.T) {
	s := new(hostMonitorSuite)
	suite.Run(t, s)
}

func (s *hostMonitorSuite) SetupSuite() {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	s.env = &mock.Environment{
		EvergreenSettings: testConfig,
	}
	s.sender = send.MakeInternalLogger()
	s.cloud = cloud.GetMockProvider()
	s.cloud.Reset()
}

func (s *hostMonitorSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(host.Collection, task.Collection))
}

func (s *hostMonitorSuite) TestExternalTermination() {
	m1 := cloud.MockInstance{
		IsUp:           true,
		IsSSHReachable: true,
		Status:         cloud.StatusTerminated,
	}
	s.cloud.Set("h1", m1)

	// this host should be picked up and updated to terminated
	h := &host.Host{
		Id: "h1",
		LastCommunicationTime: time.Now().Add(-15 * time.Minute),
		Status:                evergreen.HostRunning,
		Provider:              evergreen.ProviderNameMock,
		StartedBy:             evergreen.User,
	}
	s.Require().NoError(h.Insert())

	j := NewHostMonitorJob(s.env, h, "one")
	s.False(j.Status().Completed)

	j.Run(context.Background())

	s.NoError(j.Error())
	s.True(j.Status().Completed)

	host1, err := host.FindOne(host.ById("h1"))
	s.NoError(err)
	s.Equal(host1.Status, evergreen.HostTerminated)
}

func (s *hostMonitorSuite) TestLongRunningTasks() {
	t := task.Task{
		Id:        "task2",
		StartTime: time.Now().Add(-15 * time.Hour),
	}
	s.NoError(t.Insert())
	h := &host.Host{
		Id: "h2",
		LastCommunicationTime: time.Now(),
		Status:                evergreen.HostRunning,
		Provider:              evergreen.ProviderNameMock,
		StartedBy:             evergreen.User,
		RunningTask:           "task2",
	}
	s.NoError(h.Insert())
	m2 := cloud.MockInstance{
		IsUp:           true,
		IsSSHReachable: true,
		Status:         cloud.StatusRunning,
	}
	s.cloud.Set(h.Id, m2)

	j := makeHostMonitor()
	j.env = s.env
	j.host = h
	j.HostID = h.Id
	j.logger = logging.MakeGrip(s.sender)
	j.Run(context.Background())

	s.NoError(j.Error())
	s.True(j.Status().Completed)

	s.Require().True(s.sender.HasMessage())
	s.Contains(s.sender.GetMessage().Message.String(), "host running task for too long")
}
