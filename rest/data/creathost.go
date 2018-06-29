package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBCreateHostConnector supports `create.host` commands from the agent.
type DBCreateHostConnector struct{}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (*DBCreateHostConnector) ListHostsForTask(taskID string) ([]model.CreateHost, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, rest.APIError{StatusCode: http.StatusInternalServerError, Message: "error finding task"}
	}
	if t == nil {
		return nil, rest.APIError{StatusCode: http.StatusInternalServerError, Message: "no task found"}
	}

	catcher := grip.NewBasicCatcher()
	hostsSpawnedByTask, err := host.FindHostsSpawnedByTask(t.Id)
	catcher.Add(err)
	hostsSpawnedByBuild, err := host.FindHostsSpawnedByBuild(t.BuildId)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, rest.APIError{StatusCode: http.StatusInternalServerError, Message: catcher.String()}
	}
	hosts := []model.CreateHost{}
	for _, h := range hostsSpawnedByBuild {
		hosts = append(hosts, model.CreateHost{
			InstanceID: h.Id,
			DNSName:    h.Host,
		})
	}
	for _, h := range hostsSpawnedByTask {
		hosts = append(hosts, model.CreateHost{
			InstanceID: h.Id,
			DNSName:    h.Host,
		})
	}
	return hosts, nil
}

// MockCreateHostConnector mocks `DBCreateHostConnector`.
type MockCreateHostConnector struct{}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (*MockCreateHostConnector) ListHostsForTask(taskID string) ([]model.CreateHost, error) {
	return nil, errors.New("method not implemented")
}
