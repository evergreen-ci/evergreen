package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type hostAlertSuite struct {
	suite.Suite
	sender *send.InternalSender
}

func TestHostAlertingSuite(t *testing.T) {
	s := new(hostAlertSuite)
	suite.Run(t, s)
}

func (s *hostAlertSuite) SetupSuite() {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	s.sender = send.MakeInternalLogger()
}

func (s *hostAlertSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(host.Collection, task.Collection))
}

func (s *hostAlertSuite) TestLongRunningTasks() {
	t := task.Task{
		Id:        "task2",
		StartTime: time.Now().Add(-15 * time.Hour),
	}
	s.NoError(t.Insert())
	h := host.Host{
		Id:                    "h2",
		LastCommunicationTime: time.Now(),
		Status:                evergreen.HostRunning,
		Provider:              evergreen.ProviderNameMock,
		StartedBy:             evergreen.User,
		RunningTask:           "task2",
	}
	s.NoError(h.Insert())

	j := makeHostAlerting()
	j.host = &h
	j.HostID = h.Id
	j.logger = logging.MakeGrip(s.sender)
	j.Run(context.Background())

	s.NoError(j.Error())
	s.True(j.Status().Completed)

	s.Require().True(s.sender.HasMessage())
	s.Contains(s.sender.GetMessage().Message.String(), "host running task for too long")
}
