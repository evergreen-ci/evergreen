package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// UpdateDistro updates the given distro.Distro.
func UpdateDistro(ctx context.Context, old, new *distro.Distro) error {
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
	if err := new.ReplaceOne(ctx); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "updating distro '%s'", new.Id).Error(),
		}
	}
	return nil
}

// DeleteDistroById removes a given distro from the database based on its id.
func DeleteDistroById(ctx context.Context, distroId string) error {
	d, err := distro.FindOneId(ctx, distroId)
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
	if err = host.MarkInactiveStaticHosts(ctx, []string{}, d); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "terminating inactive static hosts in distro '%s'", distroId).Error(),
		}
	}
	if err = distro.Remove(ctx, distroId); err != nil {
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
