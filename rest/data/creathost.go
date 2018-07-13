package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBCreateHostConnector supports `host.create` commands from the agent.
type DBCreateHostConnector struct{}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (*DBCreateHostConnector) ListHostsForTask(taskID string) ([]host.Host, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: "error finding task"}
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: "no task found"}
	}

	catcher := grip.NewBasicCatcher()
	hostsSpawnedByTask, err := host.FindHostsSpawnedByTask(t.Id)
	catcher.Add(err)
	hostsSpawnedByBuild, err := host.FindHostsSpawnedByBuild(t.BuildId)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: catcher.String()}
	}
	hosts := []host.Host{}
	for _, h := range hostsSpawnedByBuild {
		hosts = append(hosts, h)
	}
	for _, h := range hostsSpawnedByTask {
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// MockCreateHostConnector mocks `DBCreateHostConnector`.
type MockCreateHostConnector struct{}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (*MockCreateHostConnector) ListHostsForTask(taskID string) ([]host.Host, error) {
	return nil, errors.New("method not implemented")
}
