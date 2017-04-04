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
	grip.CatchError(grip.SetSender(testutil.SetupTestSender(taskRunnerTestConf.TaskRunner.LogFile)))
}

type MockHostGateway struct{}

func (self *MockHostGateway) RunSetup() error {
	return errors.New("RunSetup not implemented")
}

func (self *MockHostGateway) GetAgentRevision() (string, error) {
	return "", errors.New("GetAgentRevision not implemented")
}

func (self *MockHostGateway) StartAgentOnHost(settings *evergreen.Settings,
	targetHost host.Host) (string, error) {
	return "", errors.New("RunTaskOnHost not implemented")
}

func (self *MockHostGateway) AgentNeedsBuild() (bool, error) {
	return false, errors.New("AgentNeedsBuild not implemented")
}
