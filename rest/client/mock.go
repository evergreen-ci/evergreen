package client

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"golang.org/x/net/context"
)

// MockEvergreenREST mocks EvergreenREST for testing.
type MockEvergreenREST struct {
	serverURL    string
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	hostID       string
	hostSecret   string
	httpsCert    string
	httpClient   *http.Client
}

// NewMockEvergreenREST returns a Communicator for testing.
func NewMockEvergreenREST(serverURL, hostID, hostSecret, cert string) (Communicator, error) {
	evergreen := &MockEvergreenREST{
		hostID:       hostID,
		hostSecret:   hostSecret,
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		httpsCert:    cert,
		serverURL:    serverURL,
		httpClient:   &http.Client{},
	}
	return evergreen, nil
}

// StartTask returns nil.
func (c *MockEvergreenREST) StartTask(ctx context.Context, taskID, taskSecret string) error {
	return nil
}

// EndTask returns an empty EndTaskResponse.
func (c *MockEvergreenREST) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskID, taskSecret string) (*apimodels.EndTaskResponse, error) {
	return &apimodels.EndTaskResponse{}, nil
}

// GetTask returns an empty Task.
func (c *MockEvergreenREST) GetTask(ctx context.Context, taskID, taskSecret string) (*task.Task, error) {
	return &task.Task{}, nil
}

// GetProjectRef returns an empty ProjectRef.
func (c *MockEvergreenREST) GetProjectRef(ctx context.Context, taskID, taskSecret string) (*model.ProjectRef, error) {
	return &model.ProjectRef{}, nil
}

// GetDistro returns an empty Distro.
func (c *MockEvergreenREST) GetDistro(ctx context.Context, taskID, taskSecret string) (*distro.Distro, error) {
	return &distro.Distro{}, nil
}

// GetVersion return an empty Version.
func (c *MockEvergreenREST) GetVersion(ctx context.Context, taskID, taskSecret string) (*version.Version, error) {
	return &version.Version{}, nil
}

// Heartbeat returns false, which indicates the heartbeat has succeeded.
func (c *MockEvergreenREST) Heartbeat(ctx context.Context, taskID, taskSecret string) (bool, error) {
	return false, nil
}

// FetchExpansionVars returns an empty ExpansionVars.
func (c *MockEvergreenREST) FetchExpansionVars(ctx context.Context, taskID, taskSecret string) (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{}, nil
}

// GetNextTask returns an empty NextTaskResponse.
func (c *MockEvergreenREST) GetNextTask(ctx context.Context, taskID, taskSecret string) (*apimodels.NextTaskResponse, error) {
	return &apimodels.NextTaskResponse{}, nil

}

// SetTimeoutStart sets the initial timeout for a request.
func (c *MockEvergreenREST) SetTimeoutStart(timeoutStart time.Duration) {
	c.timeoutStart = timeoutStart
}

// SetTimeoutMax sets the maximum timeout for a request.
func (c *MockEvergreenREST) SetTimeoutMax(timeoutMax time.Duration) {
	c.timeoutMax = timeoutMax
}

// SetMaxAttempts sets the number of attempts a request will be made.
func (c *MockEvergreenREST) SetMaxAttempts(attempts int) {
	c.maxAttempts = attempts
}
