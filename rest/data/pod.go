package data

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// DBPodConnector implements the pod-related methods from the connector via
// interactions with the database.
type DBPodConnector struct{}

// CreatePod creates a new pod from the given REST model and returns its ID.
func (c *DBPodConnector) CreatePod(apiPod model.APICreatePod) (*model.APICreatePodResponse, error) {
	dbPod, err := apiPod.ToService()
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "API error converting from model.APICreatePod to pod.Pod").Error(),
		}
	}

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
// to the pod.
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
	s, err := p.GetSecret()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    errors.Wrap(err, "getting pod secret").Error(),
		}
	}
	if secret != s.Value {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "pod secrets do not match",
		}
	}
	return nil
}

// FindPodByID finds a pod by the given ID. It returns a nil result if no such
// pod is found.
func (c *DBPodConnector) FindPodByID(id string) (*model.APIPod, error) {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding pod by ID").Error(),
		}
	}
	if p == nil {
		return nil, nil
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

// FindPodByExternalID finds a pod by its external identifier. It returns a nil
// result if no such pod is found.
func (c *DBPodConnector) FindPodByExternalID(id string) (*model.APIPod, error) {
	p, err := pod.FindOneByExternalID(id)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding pod by ID").Error(),
		}
	}
	if p == nil {
		return nil, nil
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

// UpdatePodStatus updates the pod status from the current status to the updated
// one.
func (c *DBPodConnector) UpdatePodStatus(id string, apiCurrent, apiUpdated restModel.APIPodStatus) error {
	current, err := apiCurrent.ToService()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "converting current status to service model").Error(),
		}
	}
	updated, err := apiUpdated.ToService()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "converting current status to service model").Error(),
		}
	}

	return pod.UpdateOneStatus(id, *current, *updated, utility.BSONTime(time.Now()))
}
