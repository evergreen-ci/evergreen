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
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// UpdateDistro updates the given distro.Distro.
func UpdateDistro(old, new *distro.Distro) error {
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

type CopyDistroOpts struct {
	DistroIdToCopy string
	NewDistroId    string
}

func CopyDistro(ctx context.Context, env evergreen.Environment, u *user.DBUser, opts CopyDistroOpts) error {
	distroToCopy, err := distro.FindOneId(opts.DistroIdToCopy)
	if err != nil {
		return errors.Wrapf(err, "finding distro '%s'", opts.DistroIdToCopy)
	}
	if distroToCopy == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro '%s' not found", opts.DistroIdToCopy),
		}
	}

	distroToCopy.Id = opts.NewDistroId
	return newDistro(ctx, env, distroToCopy, u)
}

func newDistro(ctx context.Context, env evergreen.Environment, d *distro.Distro, u *user.DBUser) error {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}

	vErrs, err := validator.CheckDistro(ctx, d, settings, true)
	if err != nil {
		return errors.Wrapf(err, "validating distro '%s'", d.Id)
	}
	if len(vErrs) != 0 {
		return errors.Errorf("validator encountered errors: '%s'", vErrs.String())
	}

	if err = d.Add(u); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "inserting distro '%s'", d.Id).Error(),
		}
	}

	event.LogDistroAdded(d.Id, u.Username(), d.NewDistroData())
	return nil
}
