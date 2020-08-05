package api

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	InvalidStatusError             = "'%v' is not a valid status"
	DecommissionStaticHostError    = "Cannot decommission static hosts"
	HostTerminationQueueingError   = "Error starting background job for host termination"
	HostUpdateError                = "Error updating host"
	HostTerminationQueueingSuccess = "Host %v successfully queued for termination"
	HostStatusUpdateSuccess        = "Host status successfully updated from '%v' to '%v'"
	HostStatusWriteConfirm         = "Successfully updated host status"
	HostRestartJasperConfirm       = "Successfully marked host as needing Jasper service restarted"
	UnrecognizedAction             = "Unrecognized action: %v"
)

var (
	validUpdateToStatuses = []string{
		evergreen.HostRunning,
		evergreen.HostQuarantined,
		evergreen.HostDecommissioned,
		evergreen.HostTerminated,
	}
)

// ModifyHostsWithPermissions performs an update on each of the given hosts
// for which the permissions allow updates on that host.
func ModifyHostsWithPermissions(hosts []host.Host, perm map[string]gimlet.Permissions, modifyHost func(h *host.Host) error) (updated int, err error) {
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if perm[h.Distro.Id][evergreen.PermissionHosts] < evergreen.HostsEdit.Value {
			continue
		}
		if err := modifyHost(&h); err != nil {
			catcher.Wrapf(err, "could not modify host '%s'", h.Id)
			continue
		}
		updated++
	}
	return updated, catcher.Resolve()
}

func ModifyHostStatus(queue amboy.Queue, h *host.Host, newStatus string, notes string, u *user.DBUser) (string, error) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	currentStatus := h.Status

	if !utility.StringSliceContains(validUpdateToStatuses, newStatus) {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf(InvalidStatusError, newStatus),
		}
	}

	if h.Provider == evergreen.ProviderNameStatic && newStatus == evergreen.HostDecommissioned {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    DecommissionStaticHostError,
		}
	}

	if newStatus == evergreen.HostTerminated {
		if !queue.Info().Started {
			return "", gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    HostTerminationQueueingError,
			}
		}

		if err := queue.Put(ctx, units.NewHostTerminationJob(env, h, true, fmt.Sprintf("terminated by %s", u.Username()))); err != nil {
			return "", gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    HostTerminationQueueingError,
			}
		}
		return fmt.Sprintf(HostTerminationQueueingSuccess, h.Id), nil
	}

	err := h.SetStatus(newStatus, u.Id, notes)
	if err != nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, HostUpdateError).Error(),
		}
	}

	return fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status), nil
}
