package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	taskRunnerTestConf = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskRunnerTestConf))
	if taskRunnerTestConf.TaskRunner.LogFile != "" {
		mci.SetLogger(taskRunnerTestConf.TaskRunner.LogFile)
	}
}

// mock implementations, for testing purposes

type MockHostFinder struct{}

func (self *MockHostFinder) FindAvailableHosts() ([]model.Host, error) {
	return nil, fmt.Errorf("FindAvailableHosts not implemented")
}

type MockTaskQueueFinder struct{}

func (self *MockTaskQueueFinder) FindTaskQueue(distroName string) (*model.TaskQueue, error) {
	return nil, fmt.Errorf("FindTaskQueue not implemented")
}

type MockHostGateway struct{}

func (self *MockHostGateway) RunSetup() error {
	return fmt.Errorf("RunSetup not implemented")
}

func (self *MockHostGateway) GetAgentRevision() (string, error) {
	return "", fmt.Errorf("GetAgentRevision not implemented")
}

func (self *MockHostGateway) RunTaskOnHost(mciSettings *mci.MCISettings,
	taskToRun model.Task, targetHost model.Host) (string, error) {
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
		hosts := []model.Host{
			model.Host{Id: hostIds[0], Distro: distroIds[0]},
			model.Host{Id: hostIds[1], Distro: distroIds[1]},
			model.Host{Id: hostIds[2], Distro: distroIds[2]},
			model.Host{Id: hostIds[3], Distro: distroIds[1]},
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
