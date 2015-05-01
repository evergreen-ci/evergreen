package taskrunner

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	taskRunnerTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskRunnerTestConf))
	if taskRunnerTestConf.TaskRunner.LogFile != "" {
		evergreen.SetLogger(taskRunnerTestConf.TaskRunner.LogFile)
	}
}

// mock implementations, for testing purposes

type MockHostFinder struct{}

func (self *MockHostFinder) FindAvailableHosts() ([]host.Host, error) {
	return nil, fmt.Errorf("FindAvailableHosts not implemented")
}

type MockTaskQueueFinder struct{}

func (self *MockTaskQueueFinder) FindTaskQueue(distroId string) (*model.TaskQueue, error) {
	return nil, fmt.Errorf("FindTaskQueue not implemented")
}

type MockHostGateway struct{}

func (self *MockHostGateway) RunSetup() error {
	return fmt.Errorf("RunSetup not implemented")
}

func (self *MockHostGateway) GetAgentRevision() (string, error) {
	return "", fmt.Errorf("GetAgentRevision not implemented")
}

func (self *MockHostGateway) RunTaskOnHost(mciSettings *evergreen.MCISettings,
	taskToRun model.Task, targetHost host.Host) (string, error) {
	return "", fmt.Errorf("RunTaskOnHost not implemented")
}

func (self *MockHostGateway) AgentNeedsBuild() (bool, error) {
	return false, fmt.Errorf("AgentNeedsBuild not implemented")
}

func TestSplitHostsByDistro(t *testing.T) {

	Convey("Splitting hosts by distro should return a map of each distro to "+
		"the hosts that are a part of it", t, func() {

		distroIds := []string{"d1", "d2", "d3"}
		hostIds := []string{"h1", "h2", "h3", "h4"}
		hosts := []host.Host{
			host.Host{Id: hostIds[0], Distro: distroIds[0]},
			host.Host{Id: hostIds[1], Distro: distroIds[1]},
			host.Host{Id: hostIds[2], Distro: distroIds[2]},
			host.Host{Id: hostIds[3], Distro: distroIds[1]},
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
