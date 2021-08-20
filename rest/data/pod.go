package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// DBPodConnector implements the pod-related methods from the connector via
// interactions with the database.
type DBPodConnector struct{}

// CreatePod creates a new pod from the given REST model and returns its ID.
func (c *DBPodConnector) CreatePod(p model.APICreatePod) (*model.APICreatePodResponse, error) {
	dbPod, err := translatePod(p)
	if err != nil {
		return nil, err
	}

	addAgentPodSettings(dbPod)

	if err := dbPod.Insert(); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("pod with id '%s' was not inserted", dbPod.ID),
		}
	}

	return &model.APICreatePodResponse{ID: dbPod.ID}, nil
}

// CheckPodSecret checks for a pod with a matching ID and secret in the
// database. It returns an error if the secret does not match the one assigned
// for the given pod ID.
func (c *DBPodConnector) CheckPodSecret(id, secret string) error {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding pod by ID").Error(),
		}
	}
	if p == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "pod does not exist",
		}
	}
	if secret != p.Secret {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "pod secrets do not match",
		}
	}
	return nil
}

// FindPodByID finds a pod by the given ID. It returns an error if the pod
// cannot be found.
func (c *DBPodConnector) FindPodByID(id string) (*model.APIPod, error) {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding pod by ID").Error(),
		}
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "pod does not exist",
		}
	}
	var apiPod model.APIPod
	if err := apiPod.BuildFromService(p); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "building pod from service model").Error(),
		}
	}
	return &apiPod, nil
}

// MockPodConnector implements the pod-related methods from the connector via an
// in-memory cache of pods.
type MockPodConnector struct {
	CachedPods []pod.Pod
}

func (c *MockPodConnector) CreatePod(apiPod model.APICreatePod) (*model.APICreatePodResponse, error) {
	dbPod, err := translatePod(apiPod)
	if err != nil {
		return nil, err
	}

	addAgentPodSettings(dbPod)

	c.CachedPods = append(c.CachedPods, *dbPod)

	return &model.APICreatePodResponse{ID: dbPod.ID}, nil
}

// CheckPodSecret checks the cache for a matching pod by ID and verifies that
// the secret matches.
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

// translatePod translates a pod creation request into its data model.
func translatePod(p model.APICreatePod) (*pod.Pod, error) {
	i, err := p.ToService()
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "API error converting from model.APICreatePod to pod.Pod").Error(),
		}
	}

	dbPod, ok := i.(pod.Pod)
	if !ok {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for pod.Pod", i),
		}
	}

	return &dbPod, nil
}

// addAgentPodSettings adds any pod configuration that is necessary to run the
// agent.
func addAgentPodSettings(p *pod.Pod) {
	if p.Secret == "" {
		p.Secret = utility.RandomString()
	}
	if p.TaskContainerCreationOpts.EnvSecrets == nil {
		p.TaskContainerCreationOpts.EnvSecrets = map[string]string{}
	}
	p.TaskContainerCreationOpts.EnvSecrets["POD_SECRET"] = p.Secret
	if p.TaskContainerCreationOpts.EnvVars == nil {
		p.TaskContainerCreationOpts.EnvVars = map[string]string{}
	}
	p.TaskContainerCreationOpts.EnvVars["POD_ID"] = p.ID
}

// FindPodByID checks the cache for a matching pod by ID.
func (c *MockPodConnector) FindPodByID(id string) (*model.APIPod, error) {
	for _, p := range c.CachedPods {
		if id == p.ID {
			var apiPod model.APIPod
			if err := apiPod.BuildFromService(&p); err != nil {
				return nil, errors.Wrap(err, "building pod from service model")
			}
			return &apiPod, nil
		}
	}
	return nil, errors.New("pod does not exist")
}
