package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBPodConnector implements the pod-related methods from the connector via
// interactions with the database.
type DBPodConnector struct{}

// CreatePod inserts the given pod.Pod.
func (c *DBPodConnector) CreatePod(pod *pod.Pod) error {
	if err := pod.Insert(); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("pod with id '%s' was not inserted", pod.ID),
		}
	}
	return nil
}

// CheckPodSecret checks for a pod with a matching ID and secret in the
// database.
func (c *DBPodConnector) CheckPodSecret(id, secret string) error {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("pod does not exist")
	}
	if secret != p.Secret {
		return errors.New("incorrect pod secret")
	}
	return nil
}

// MockPodConnector implements the pod-related methods from the connector via an
// in-memory cache of pods.
type MockPodConnector struct {
	CachedPods []pod.Pod
}

func (c *MockPodConnector) CreatePod(pod *pod.Pod) error {
	for _, p := range c.CachedPods {
		if p.ID == pod.ID {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("pod with id '%s' was not inserted", pod.ID),
			}
		}
	}
	return nil
}

func (c *MockPodConnector) CheckPodSecret(id, secret string) error {
	for _, p := range c.CachedPods {
		if id != p.ID {
			continue
		}
		if secret != p.Secret {
			return errors.New("incorrect pod secret")
		}
	}
	return errors.New("pod does not exist")
}
