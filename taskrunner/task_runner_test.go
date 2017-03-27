package taskrunner

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

var taskRunnerTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskRunnerTestConf))
	grip.SetSender(testutil.SetupTestSender(taskRunnerTestConf.TaskRunner.LogFile))
}

// mock implementations, for testing purposes

type MockHostFinder struct{}

func (*MockHostFinder) FindAvailableHosts() ([]host.Host, error) {
	return nil, errors.New("FindAvailableHosts not implemented")
}

func (*MockHostFinder) FindAvailableHostsForDistro(_ string) ([]host.Host, error) {
	return nil, errors.New("FindAvailableHostsForDistro not implemented")
}

type MockTaskQueueFinder struct{}

func (*MockTaskQueueFinder) FindTaskQueue(distroId string) (*model.TaskQueue, error) {
	return nil, errors.New("FindTaskQueue not implemented")
}

type MockHostGateway struct{}

func (self *MockHostGateway) RunSetup() error {
	return errors.New("RunSetup not implemented")
}

func (self *MockHostGateway) GetAgentRevision() (string, error) {
	return "", errors.New("GetAgentRevision not implemented")
}

func (self *MockHostGateway) RunTaskOnHost(settings *evergreen.Settings,
	taskToRun task.Task, targetHost host.Host) (string, error) {
	return "", errors.New("RunTaskOnHost not implemented")
}

func (self *MockHostGateway) AgentNeedsBuild() (bool, error) {
	return false, errors.New("AgentNeedsBuild not implemented")
}

func TestSplitHostsByDistro(t *testing.T) {

	Convey("Splitting hosts by distro should return a map of each distro to "+
		"the hosts that are a part of it", t, func() {

		distroIds := []string{"d1", "d2", "d3"}
		hostIds := []string{"h1", "h2", "h3", "h4"}
		hosts := []host.Host{
			{Id: hostIds[0], Distro: distro.Distro{Id: distroIds[0]}},
			{Id: hostIds[1], Distro: distro.Distro{Id: distroIds[1]}},
			{Id: hostIds[2], Distro: distro.Distro{Id: distroIds[2]}},
			{Id: hostIds[3], Distro: distro.Distro{Id: distroIds[1]}},
		}

		taskRunner := &TaskRunner{
			taskRunnerTestConf,
			&MockHostFinder{},
			&MockTaskQueueFinder{},
			&MockHostGateway{},
		}

		splitHosts := taskRunner.splitHostsByDistro(hosts)
		distroZeroHosts := splitHosts[distroIds[0]]
		distroOneHosts := splitHosts[distroIds[1]]
		distroTwoHosts := splitHosts[distroIds[2]]
		So(len(distroZeroHosts), ShouldEqual, 1)
		So(distroZeroHosts[0].Id, ShouldEqual, hostIds[0])
		So(len(distroOneHosts), ShouldEqual, 2)
		So(distroOneHosts[0].Id, ShouldEqual, hostIds[1])
		So(distroOneHosts[1].Id, ShouldEqual, hostIds[3])
		So(len(distroTwoHosts), ShouldEqual, 1)
		So(distroTwoHosts[0].Id, ShouldEqual, hostIds[2])

	})
}
