package model

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

func ValidateTask(taskId string, checkSecret bool, r *http.Request) (*task.Task, int, error) {
	if taskId == "" {
		return nil, http.StatusBadRequest, errors.New("missing task id")
	}
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if t == nil {
		return nil, http.StatusNotFound, errors.New("task not found")
	}
	if checkSecret {
		secret := r.Header.Get(evergreen.TaskSecretHeader)
		if secret != t.Secret {
			return nil, http.StatusConflict, errors.Errorf("Wrong secret sent for task %s: Expected %s but got %s",
				taskId, t.Secret, secret)
		}
	}
	return t, http.StatusOK, nil
}

func ValidateHost(hostId string, r *http.Request) (*host.Host, int, error) {
	if hostId == "" {
		// fall back to the host header if host ids are not part of the path
		hostId = r.Header.Get(evergreen.HostHeader)
		if hostId == "" {
			return nil, http.StatusBadRequest, errors.Errorf("Request %s is missing host information", r.URL)
		}
	}
	secret := r.Header.Get(evergreen.HostSecretHeader)

	h, err := host.FindOne(host.ById(hostId))
	if h == nil {
		return nil, http.StatusBadRequest, errors.Errorf("Host %s not found", hostId)
	}
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "Error loading context for host %v", hostId)
	}
	// if there is a secret, ensure we are using the correct one -- fail if we arent
	if secret != "" && secret != h.Secret {
		return nil, http.StatusConflict, errors.Errorf("Invalid host secret for host %v", h.Id)
	}

	// if the task is attached to the context, check host-task relationship
	var t *task.Task
	if rv := r.Context().Value(0); rv != nil {
		if rvTask, ok := rv.(*task.Task); ok {
			t = rvTask
		}
	}
	if badHostTaskRelationship(h, t) {
		return nil, http.StatusConflict, errors.Errorf("Host %v should be running %v, not %v", h.Id, h.RunningTask, t.Id)
	}
	return h, http.StatusOK, nil
}

func badHostTaskRelationship(h *host.Host, t *task.Task) bool {
	if t == nil {
		return false
	}
	if t.Id == h.RunningTask {
		return false
	}
	if t.Id == h.LastTaskCompleted && h.RunningTask == "" {
		return false
	}
	return true
}
