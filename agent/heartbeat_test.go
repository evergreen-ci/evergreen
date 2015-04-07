package agent

import (
	"10gen.com/mci/apimodels"
	"10gen.com/mci/model"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/patch"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"testing"
	"time"
)

type MockCommunicator struct {
	shouldFailStart     bool
	shouldFailEnd       bool
	shouldFailHeartbeat bool
	abort               bool
	logChan             chan []model.LogMessage
}

func (self *MockCommunicator) tryGet(path string) (*http.Response, error) {
	return nil, nil
}

func (self *MockCommunicator) tryPostJSON(path string, data interface{}) (*http.Response, error) {
	return nil, nil
}

func (self *MockCommunicator) Start(pid string) error {
	if self.shouldFailStart {
		return fmt.Errorf("failed to start!")
	}
	return nil
}

func (self *MockCommunicator) End(status string,
	details *apimodels.TaskEndDetails) (*apimodels.TaskEndResponse, error) {
	if self.shouldFailEnd {
		return nil, fmt.Errorf("failed to end!")
	}
	return nil, nil
}

func (self *MockCommunicator) GetTask() (*model.Task, error) {
	return &model.Task{}, nil
}

func (self *MockCommunicator) GetPatch() (*patch.Patch, error) {
	return &patch.Patch{}, nil
}

func (self *MockCommunicator) GetDistro() (*distro.Distro, error) {
	return &distro.Distro{}, nil
}

func (self *MockCommunicator) GetProjectConfig() (*model.Project, error) {
	return &model.Project{}, nil
}

func (self *MockCommunicator) Log(logMessages []model.LogMessage) error {
	if self.shouldFailEnd {
		return fmt.Errorf("failed to end!")
	}
	self.logChan <- logMessages
	return nil
}

func (self *MockCommunicator) Heartbeat() (bool, error) {
	if self.shouldFailHeartbeat {
		return false, fmt.Errorf("failed to heartbeat!")
	}
	return self.abort, nil
}

func (self *MockCommunicator) FetchExpansionVars() (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{}, nil
}

func TestHeartbeat(t *testing.T) {

	Convey("With a simple heartbeat ticker", t, func() {
		sigChan := make(chan AgentSignal)
		mockCommunicator := &MockCommunicator{}
		hbTicker := &HeartbeatTicker{
			MaxFailedHeartbeats: 10,
			Interval:            10 * time.Millisecond,
			SignalChan:          sigChan,
			TaskCommunicator:    mockCommunicator,
			Logger: &slogger.Logger{
				Appenders: []slogger.Appender{},
			},
		}

		Convey("abort signals detected by heartbeat are sent on sigChan", func() {
			mockCommunicator.shouldFailHeartbeat = false
			hbTicker.StartHeartbeating()
			go func() {
				time.Sleep(2 * time.Second)
				mockCommunicator.abort = true
			}()
			signal := <-sigChan
			So(signal, ShouldEqual, AbortedByUser)
		})

		Convey("failed heartbeats must signal failure on sigChan", func() {
			mockCommunicator.abort = false
			mockCommunicator.shouldFailHeartbeat = true
			hbTicker.StartHeartbeating()
			signal := <-sigChan
			So(signal, ShouldEqual, HeartbeatMaxFailed)
		})

	})

}
