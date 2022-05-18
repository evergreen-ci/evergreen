package data

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// CreatePod creates a new pod from the given REST model and returns its ID.
func CreatePod(apiPod model.APICreatePod) (*model.APICreatePodResponse, error) {
	dbPod, err := apiPod.ToService()
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "converting pod to service model").Error(),
		}
	}

	if err := dbPod.Insert(); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "inserting new intent pod '%s'", dbPod.ID).Error(),
		}
	}

	return &model.APICreatePodResponse{ID: dbPod.ID}, nil
}

// CheckPodSecret checks for a pod with a matching ID and secret in the
// database. It returns an error if the secret does not match the one assigned
// to the pod.
func CheckPodSecret(id, secret string) error {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding pod '%s'", id).Error(),
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
func FindPodByID(id string) (*model.APIPod, error) {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding pod '%s'", id).Error(),
		}
	}
	if p == nil {
		return nil, nil
	}
	var apiPod model.APIPod
	if err := apiPod.BuildFromService(p); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "converting pod to API model").Error(),
		}
	}
	return &apiPod, nil
}

// FindPodByExternalID finds a pod by its external identifier. It returns a nil
// result if no such pod is found.
func FindPodByExternalID(id string) (*model.APIPod, error) {
	p, err := pod.FindOneByExternalID(id)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding pod with external ID '%s'", id).Error(),
		}
	}
	if p == nil {
		return nil, nil
	}

	var apiPod model.APIPod
	if err := apiPod.BuildFromService(p); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "converting pod to API model").Error(),
		}
	}
	return &apiPod, nil
}

// UpdatePodStatus updates the pod status from the current status to the updated
// one.
func UpdatePodStatus(id string, apiCurrent, apiUpdated restModel.APIPodStatus) error {
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
