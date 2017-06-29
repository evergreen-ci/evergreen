package comm

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip/slogger"
)

// TaskCommunicator is an interface that handles the remote procedure calls
// between an agent and the remote server.
type TaskCommunicator interface {
	Start() error
	End(detail *apimodels.TaskEndDetail) (*apimodels.EndTaskResponse, error)
	GetTask() (*task.Task, error)
	GetProjectRef() (*model.ProjectRef, error)
	GetDistro() (*distro.Distro, error)
	GetVersion() (*version.Version, error)
	Log([]apimodels.LogMessage) error
	Heartbeat() (bool, error)
	FetchExpansionVars() (*apimodels.ExpansionVars, error)
	GetNextTask() (*apimodels.NextTaskResponse, error)
	TryTaskGet(path string) (*http.Response, error)
	TryTaskPost(path string, data interface{}) (*http.Response, error)
	TryGet(path string) (*http.Response, error)
	TryPostJSON(path string, data interface{}) (*http.Response, error)
	SetTask(taskId, taskSecret string)
	GetCurrentTaskId() string
	SetSignalChan(chan Signal)
	SetLogger(*slogger.Logger)
}
