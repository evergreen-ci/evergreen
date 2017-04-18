package taskrunner

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
)

var taskRunnerTestConf = testutil.TestConfig()
var agtRevision = "abc"

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskRunnerTestConf))
	grip.CatchError(grip.SetSender(testutil.SetupTestSender(taskRunnerTestConf.TaskRunner.LogFile)))
}

type MockHostGateway struct{}

func (self *MockHostGateway) RunSetup() error {
	return nil
}

func (self *MockHostGateway) GetAgentRevision() (string, error) {
	return agtRevision, nil
}

func (self *MockHostGateway) StartAgentOnHost(settings *evergreen.Settings,
	targetHost host.Host) error {
	return agtRevision, nil
}

func (self *MockHostGateway) AgentNeedsBuild() (bool, error) {
	return false, nil
}

func TestTaskRunner(t *testing.T) {
	Convey("with a mocked task runner and a free host", t, func() {
		if err := db.ClearCollections(host.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		tr := TaskRunner{
			taskRunnerTestConf,
			nil,
			nil,
			&MockHostGateway{},
		}

		h1 := host.Host{
			Id:          "host1",
			Status:      evergreen.HostRunning,
			RunningTask: "tid",
		}
		So(h1.Insert(), ShouldBeNil)

		Convey("running the task runner should modify the host's revision", func() {
			So(tr.Run(), ShouldBeNil)
			h, err := host.FindOne(host.ById(h1.Id))
			So(err, ShouldBeNil)
			So(h.AgentRevision, ShouldEqual, agtRevision)
		})

	})
}
