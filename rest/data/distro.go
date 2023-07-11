package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// ValidateDistro checks that the changes being made to the distro with the given resourceID are valid.
func ValidateDistro(ctx context.Context, apiDistro *restModel.APIDistro, resourceID string, settings *evergreen.Settings, isNewDistro bool) (*distro.Distro, error) {
	d := apiDistro.ToService()

	id := utility.FromStringPtr(apiDistro.Name)
	if resourceID != id {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("distro name '%s' is immutable so it cannot be renamed to '%s'", resourceID, id),
		}
	}

	vErrors, err := validator.CheckDistro(ctx, d, settings, isNewDistro)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	if len(vErrors) != 0 {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    vErrors.String(),
		}
	}

	return d, nil
}

// UpdateDistro updates the given distro.Distro.
func UpdateDistro(old, new *distro.Distro, userID string) error {
	if old.Id != new.Id {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("old distro '%s' and new distro '%s' have mismatched IDs", new.Id, old.Id),
		}
	}

	if old.DispatcherSettings.Version == evergreen.DispatcherVersionRevisedWithDependencies && new.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
		if err := model.RemoveTaskQueues(new.Id); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "removing task queues for distro '%s'", new.Id).Error(),
			}
		}
	}
	err := new.Update()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "updating distro '%s'", new.Id).Error(),
		}
	}

	if old.GetDefaultAMI() != new.GetDefaultAMI() {
		event.LogDistroAMIModified(old.Id, userID)
	}
	event.LogDistroModified(old.Id, userID, new.NewDistroData())

	return nil
}

// DeleteDistroById removes a given distro from the database based on its id.
func DeleteDistroById(distroId string) error {
	d, err := distro.FindOneId(distroId)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding distro '%s'", distroId).Error(),
		}
	}
	if d == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("distro '%s' not found", distroId),
		}
	}
	if err = host.MarkInactiveStaticHosts([]string{}, d); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "terminating inactive static hosts in distro '%s'", distroId).Error(),
		}
	}
	if err = distro.Remove(distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "deleting distro '%s'", distroId).Error(),
		}
	}
	if err = model.ClearTaskQueue(distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "clearing task queue for distro '%s'", distroId).Error(),
		}
	}
	return nil
}
