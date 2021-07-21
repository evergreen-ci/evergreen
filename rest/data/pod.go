package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/pod"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBPodConnector implements the pod-related methods from the connector via
// interactions with the database.
type DBPodConnector struct{}

// CreatePod inserts the given pod.Pod.
func (c *DBPodConnector) CreatePod(p restModel.APICreatePod) (string, error) {
	podDB, err := validatePod(p)
	if err != nil {
		return "", err
	}

	if err := podDB.Insert(); err != nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("pod with id '%s' was not inserted", podDB.ID),
		}
	}

	return podDB.ID, nil
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

func (c *MockPodConnector) CreatePod(apiPod restModel.APICreatePod) (string, error) {
	for _, p := range c.CachedPods {
		if p.ID == *apiPod.Name {
			return "", gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("duplicate pod with id '%s' was not inserted", *apiPod.Name),
			}
		}
	}

	podDB, err := validatePod(apiPod)
	if err != nil {
		return "", err
	}
	c.CachedPods = append(c.CachedPods, *podDB)

	return podDB.ID, nil
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

func validatePod(p restModel.APICreatePod) (*pod.Pod, error) {
	if p.Image == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Invalid API input: missing image"),
		}
	}

	for _, envVar := range p.EnvVars {
		if envVar.SecretOpts == nil {
			if envVar.Name == nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    fmt.Sprintf("Invalid API input: missing environment variable name"),
				}
			}
		} else {
			if envVar.SecretOpts.Name == nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    fmt.Sprintf("Invalid API input: missing secret name"),
				}
			}
		}
	}

	i, err := p.ToService()
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("API error converting from model.APICreatePod to pod.Pod"),
		}
	}

	podDB, ok := i.(pod.Pod)
	if !ok {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for pod.Pod", i),
		}
	}

	return &podDB, nil
}
