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
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/validator"
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
		if err := model.RemoveTaskQueues(ctx, new.Id); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "removing task queues for distro '%s'", new.Id).Error(),
			}
		}
	}

	if !old.Disabled && new.Disabled {
		if err := model.ClearTaskQueue(ctx, new.Id); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "clearing task queues for distro '%s'", new.Id).Error(),
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
func DeleteDistroById(ctx context.Context, u *user.DBUser, distroId string) error {
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
	if err = model.ClearTaskQueue(ctx, distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "clearing task queue for distro '%s'", distroId).Error(),
		}
	}

	event.LogDistroRemoved(ctx, d.Id, u.Username(), d.DistroData())
	return nil
}

// CopyDistro duplicates a given distro in the database given options specifying the existing and new distro ID.
// It returns an error if one is encountered.
func CopyDistro(ctx context.Context, u *user.DBUser, opts restModel.CopyDistroOpts) error {
	if opts.DistroIdToCopy == opts.NewDistroId {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "new and existing distro IDs are identical",
		}
	}
	distroToCopy, err := distro.FindOneId(ctx, opts.DistroIdToCopy)
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
	distroToCopy.Aliases = nil
	return NewDistro(ctx, distroToCopy, u)
}

// CreateDistro creates a new distro with the provided ID using the default settings specified here.
// It returns an error if one is encountered.
func CreateDistro(ctx context.Context, u *user.DBUser, newDistroId string, singleTaskDistro bool) error {
	defaultDistro := &distro.Distro{
		Id:   newDistroId,
		Arch: evergreen.ArchLinuxAmd64,
		BootstrapSettings: distro.BootstrapSettings{
			Method:        distro.BootstrapMethodLegacySSH,
			Communication: distro.CommunicationMethodLegacySSH,
		},
		DispatcherSettings: distro.DispatcherSettings{
			Version: evergreen.DispatcherVersionRevisedWithDependencies,
		},
		FinderSettings: distro.FinderSettings{
			Version: evergreen.FinderVersionLegacy,
		},
		HostAllocatorSettings: distro.HostAllocatorSettings{
			Version:              evergreen.HostAllocatorUtilization,
			AutoTuneMaximumHosts: true,
		},
		PlannerSettings: distro.PlannerSettings{
			Version: evergreen.PlannerVersionTunable,
		},
		Provider:         evergreen.ProviderNameStatic,
		SingleTaskDistro: singleTaskDistro,
		User:             "ubuntu",
		WorkDir:          "/data/mci",
	}

	return NewDistro(ctx, defaultDistro, u)
}

// NewDistro creates a new distro in the database with the given user as the creator and creates an event log.
func NewDistro(ctx context.Context, d *distro.Distro, u *user.DBUser) error {
	settings, err := evergreen.GetConfig(ctx)
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

	if err = d.Add(ctx, u); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "inserting distro '%s'", d.Id).Error(),
		}
	}

	event.LogDistroAdded(ctx, d.Id, u.Username(), d.DistroData())
	return nil
}
