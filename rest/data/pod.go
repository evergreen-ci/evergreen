package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// CreatePod creates a new pod from the given REST model and returns its ID.
func CreatePod(apiPod model.APICreatePod) (*model.APICreatePodResponse, error) {
	if apiPod.PodSecretValue == nil {
		env := evergreen.GetEnvironment()
		smClient, err := cloud.MakeSecretsManagerClient(env.Settings())
		if err != nil {
			return nil, errors.Wrap(err, "getting Secrets Manager client")
		}
		v, err := cloud.MakeSecretsManagerVault(smClient)
		if err != nil {
			return nil, errors.Wrap(err, "initializing Secrets Manager vault")
		}
		ctx, cancel := env.Context()
		defer cancel()
		podSecret, err := v.GetValue(ctx, utility.FromStringPtr(apiPod.PodSecretExternalID))
		if err != nil {
			return nil, errors.Wrap(err, "getting pod secret value")
		}
		apiPod.PodSecretValue = utility.ToStringPtr(podSecret)
	}

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

func FindPod(podID string) (*pod.Pod, error) {
	p, err := pod.FindOneByID(podID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding pod").Error(),
		}
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("pod '%s' not found", podID),
		}
	}

	return p, nil
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
