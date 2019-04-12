package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
)

const testDistroID = "test"

type SchedulerConnectorSuite struct {
	suite.Suite
	scheduler *Scheduler
}

func TestSchedulerSuite(t *testing.T) {
	s := new(SchedulerConnectorSuite)
	s.scheduler = &Scheduler{}

	suite.Run(t, s)
}

func (s *SchedulerConnectorSuite) TestFindUsableHosts() {
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	s.NotNil(session)
	s.NoError(session.DB(schedulerTestConf.Database.DB).DropDatabase())

	distro := distro.Distro{
		Id: testDistroID,
	}
	uninitializedHost := &host.Host{Id: "uninitializedHost", Distro: distro, Status: evergreen.HostStarting, StartedBy: evergreen.User}
	startingHost := &host.Host{Id: "startingHost", Distro: distro, Status: evergreen.HostStarting, StartedBy: evergreen.User}
	runningHost := &host.Host{Id: "runningHost", Distro: distro, Status: evergreen.HostRunning, StartedBy: evergreen.User}
	terminatedHost := &host.Host{Id: "terminatedHost", Distro: distro, Status: evergreen.HostTerminated, StartedBy: evergreen.User}

	s.NoError(uninitializedHost.Insert())
	s.NoError(startingHost.Insert())
	s.NoError(runningHost.Insert())
	s.NoError(terminatedHost.Insert())

	hostMap, err := host.AllRunningHosts("")
	s.NoError(err)
	s.NotNil(hostMap)

	for _, foundHost := range hostMap {
		s.NotEqual(evergreen.HostTerminated, foundHost.Status)
	}
	s.Len(hostMap, 3)
}
