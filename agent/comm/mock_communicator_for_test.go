package comm

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
)

type MockCommunicator struct {
	shouldFailStart     bool
	shouldFailEnd       bool
	shouldFailHeartbeat bool
	abort               bool
	logChan             chan []model.LogMessage
	sync.RWMutex
}

func (*MockCommunicator) TryGet(path string) (*http.Response, error) {
	return nil, nil
}

func (*MockCommunicator) TryPostJSON(path string, data interface{}) (*http.Response, error) {
	return nil, nil
}

func (mc *MockCommunicator) Start(pid string) error {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailStart {
		return fmt.Errorf("failed to start!")
	}
	return nil
}

func (mc *MockCommunicator) End(details *apimodels.TaskEndDetail) (*apimodels.TaskEndResponse, error) {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailEnd {
		return nil, fmt.Errorf("failed to end!")
	}
	return nil, nil
}

func (*MockCommunicator) GetTask() (*task.Task, error) {
	return &task.Task{}, nil
}

func (*MockCommunicator) GetDistro() (*distro.Distro, error) {
	return &distro.Distro{}, nil
}

func (*MockCommunicator) GetProjectRef() (*model.ProjectRef, error) {
	return &model.ProjectRef{}, nil
}

func (*MockCommunicator) GetVersion() (*version.Version, error) {
	return &version.Version{}, nil
}

func (mc *MockCommunicator) setAbort(b bool) {
	mc.Lock()
	defer mc.Unlock()

	mc.abort = b
}

func (mc *MockCommunicator) setShouldFail(b bool) {
	mc.Lock()
	defer mc.Unlock()

	mc.shouldFailHeartbeat = b
}

func (mc *MockCommunicator) Log(logMessages []model.LogMessage) error {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailEnd {
		return fmt.Errorf("failed to end!")
	}
	mc.logChan <- logMessages
	return nil
}

func (mc *MockCommunicator) Heartbeat() (bool, error) {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailHeartbeat {
		return false, fmt.Errorf("failed to heartbeat!")
	}
	return mc.abort, nil
}

func (*MockCommunicator) FetchExpansionVars() (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{}, nil
}
