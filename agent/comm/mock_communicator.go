package comm

import (
	"net/http"
	"sync"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type MockCommunicator struct {
	shouldFailStart     bool
	shouldFailEnd       bool
	shouldFailHeartbeat bool
	abort               bool
	TaskId              string
	TaskSecret          string
	LogChan             chan []model.LogMessage
	Posts               map[string][]interface{}
	sync.RWMutex
}

func (mc *MockCommunicator) SetTask(taskId, taskSecret string) {

	mc.TaskId = taskId
	mc.TaskSecret = taskSecret
}

func (mc *MockCommunicator) GetCurrentTaskId() string {
	return mc.TaskId
}

func (mc *MockCommunicator) Reset(commSignal chan Signal, timeoutWatcher *TimeoutWatcher) (*APILogger, *StreamLogger, error) {
	return nil, nil, nil
}

func (*MockCommunicator) TryGet(path string) (*http.Response, error) {
	return nil, nil
}

func (*MockCommunicator) TryTaskGet(path string) (*http.Response, error) {
	return nil, nil
}

func (mc *MockCommunicator) TryPostJSON(path string, data interface{}) (*http.Response, error) {
	mc.Lock()
	defer mc.Unlock()

	if mc.Posts != nil {
		mc.Posts[path] = append(mc.Posts[path], data)
	} else {
		grip.Warningf("post to %s is not persisted in the path")
	}

	return nil, nil
}

func (mc *MockCommunicator) TryTaskPost(path string, data interface{}) (*http.Response, error) {
	mc.Lock()
	defer mc.Unlock()

	if mc.Posts != nil {
		mc.Posts[path] = append(mc.Posts[path], data)
	} else {
		grip.Warningf("post to %s is not persisted in the path")
	}

	return nil, nil
}

func (mc *MockCommunicator) Start() error {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailStart {
		return errors.New("failed to start")
	}
	return nil
}

func (mc *MockCommunicator) End(details *apimodels.TaskEndDetail) (*apimodels.EndTaskResponse, error) {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailEnd {
		return nil, errors.New("failed to end")
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

func (*MockCommunicator) GetNextTask() (*apimodels.NextTaskResponse, error) {
	return &apimodels.NextTaskResponse{}, nil
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
		return errors.New("failed to end")
	}

	if mc.LogChan != nil {
		mc.LogChan <- logMessages
	} else {
		grip.Warningf("passing over %d log messages because un-configured channel",
			len(logMessages))
	}

	return nil
}

func (mc *MockCommunicator) Heartbeat() (bool, error) {
	mc.RLock()
	defer mc.RUnlock()

	if mc.shouldFailHeartbeat {
		return false, errors.New("failed to heartbeat")
	}
	return mc.abort, nil
}

func (*MockCommunicator) FetchExpansionVars() (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{}, nil
}
