package client

import (
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"golang.org/x/net/context"
)

// Communicator is an interface for communicating with the API server.
type Communicator interface {
	// StartTask marks the task as started.
	StartTask(context.Context, string, string) error
	// EndTask marks the task as finished with the given status
	EndTask(context.Context, *apimodels.TaskEndDetail, string, string) (*apimodels.EndTaskResponse, error)
	// GetTask returns the active task.
	GetTask(context.Context, string, string) (*task.Task, error)
	// GetProjectRef loads the task's project.
	GetProjectRef(context.Context, string, string) (*model.ProjectRef, error)
	// GetDistro returns the distro for the task.
	GetDistro(context.Context, string, string) (*distro.Distro, error)
	// GetVersion loads the task's version.
	GetVersion(context.Context, string, string) (*version.Version, error)
	// Heartbeat sends a heartbeat to the API server. The server can respond with
	// an "abort" response. This function returns true if the agent should abort.
	Heartbeat(context.Context, string, string) (bool, error)
	// FetchExpansionVars loads expansions for a communicator's task from the API server.
	FetchExpansionVars(context.Context, string, string) (*apimodels.ExpansionVars, error)
	// GetNextTask returns a next task response by getting the next task for a given host.
	GetNextTask(context.Context, string, string) (*apimodels.NextTaskResponse, error)
	// SetTimeoutStart sets the initial timeout for a request.
	SetTimeoutStart(timeoutStart time.Duration)
	// SetTimeoutMax sets the maximum timeout for a request.
	SetTimeoutMax(timeoutMax time.Duration)
	// SetMaxAttempts sets the number of attempts a request will be made.
	SetMaxAttempts(attempts int)
}
