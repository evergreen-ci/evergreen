// Package api provides common functions used by the service and graphql packages.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	HostReprovisionConfirm         = "Successfully marked host as needing to reprovision"
)

var (
	validUpdateToStatuses = []string{
		evergreen.HostRunning,
		evergreen.HostStopped,
		evergreen.HostQuarantined,
		evergreen.HostDecommissioned,
		evergreen.HostTerminated,
	}
)

func GetHostsAndUserPermissions(ctx context.Context, user *user.DBUser, hostIds []string) ([]host.Host, map[string]gimlet.Permissions, int, error) {
	if len(hostIds) == 0 {
		return nil, nil, http.StatusBadRequest, errors.New("hostIds cannot be empty")
	}

	hosts, err := host.Find(ctx, host.ByIds(hostIds))
	if err != nil {
		return nil, nil, http.StatusInternalServerError, errors.New("Error getting hosts to update")
	}
	if len(hosts) == 0 {
		return nil, nil, http.StatusNotFound, errors.New("No matching hosts found")
	}

	var permissions map[string]gimlet.Permissions

	rm := evergreen.GetEnvironment().RoleManager()

	permissions, err = rolemanager.HighestPermissionsForRolesAndResourceType(user.Roles(), evergreen.DistroResourceType, rm)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, errors.New("unable to get user permissions")
	}

	return hosts, permissions, http.StatusOK, nil
}

// ModifyHostsWithPermissions performs an update on each of the given hosts
// for which the permissions allow updates on that host.
func ModifyHostsWithPermissions(hosts []host.Host, perm map[string]gimlet.Permissions, modifyHost func(h *host.Host) (int, error)) (updated int, httpStatus int, err error) {
	catcher := grip.NewBasicCatcher()

	httpStatus = http.StatusOK

	for _, h := range hosts {
		if perm[h.Distro.Id][evergreen.PermissionHosts] < evergreen.HostsEdit.Value {
			continue
		}
		if hs, err := modifyHost(&h); err != nil {
			httpStatus = hs
			catcher.Wrapf(err, "could not modify host '%s'", h.Id)
			continue
		}
		updated++
	}

	return updated, httpStatus, catcher.Resolve()
}

func ModifyHostStatus(ctx context.Context, env evergreen.Environment, h *host.Host, newStatus string, notes string, u *user.DBUser) (string, int, error) {
	currentStatus := h.Status

	if !utility.StringSliceContains(validUpdateToStatuses, newStatus) {
		return "", http.StatusBadRequest, errors.Errorf(InvalidStatusError, newStatus)
	}

	if h.Provider == evergreen.ProviderNameStatic && newStatus == evergreen.HostDecommissioned {
		return "", http.StatusBadRequest, errors.New(DecommissionStaticHostError)
	}

	if newStatus == evergreen.HostQuarantined {
		if err := units.DisableAndNotifyPoisonedHost(ctx, env, h, false, notes); err != nil {
			return "", http.StatusInternalServerError, errors.Wrap(err, HostUpdateError)
		}
		return fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status), http.StatusOK, nil
	}

	if newStatus == evergreen.HostTerminated {
		reason := notes
		if reason == "" {
			reason = fmt.Sprintf("terminated by user '%s'", u.Username())
		}
		if err := units.EnqueueTerminateHostJob(ctx, env, units.NewHostTerminationJob(env, h, units.HostTerminationOptions{
			TerminateIfBusy:   true,
			TerminationReason: reason,
		})); err != nil {
			return "", http.StatusInternalServerError, errors.New(HostTerminationQueueingError)
		}
		return fmt.Sprintf(HostTerminationQueueingSuccess, h.Id), http.StatusOK, nil
	}

	err := h.SetStatus(ctx, newStatus, u.Id, notes)
	if err != nil {
		return "", http.StatusInternalServerError, errors.Wrap(err, HostUpdateError)
	}

	unquarantinedAndNeedsReprovision := utility.StringSliceContains([]string{distro.BootstrapMethodSSH, distro.BootstrapMethodUserData}, h.Distro.BootstrapSettings.Method) &&
		currentStatus == evergreen.HostQuarantined &&
		utility.StringSliceContains([]string{evergreen.HostRunning, evergreen.HostProvisioning}, newStatus)
	if unquarantinedAndNeedsReprovision {
		if _, err = GetReprovisionToNewCallback(ctx, env, u.Username())(h); err != nil {
			return "", http.StatusInternalServerError, errors.Wrap(err, HostUpdateError)
		}
		if err = h.UnsetNumAgentCleanupFailures(ctx); err != nil {
			return "", http.StatusInternalServerError, errors.Wrap(err, HostUpdateError)
		}

		return HostReprovisionConfirm, http.StatusOK, nil
	}

	return fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status), http.StatusOK, nil
}

func GetRestartJasperCallback(ctx context.Context, env evergreen.Environment, username string) func(h *host.Host) (int, error) {
	return func(h *host.Host) (int, error) {
		modifyErr := h.SetNeedsToRestartJasper(ctx, username)
		if modifyErr != nil {
			return http.StatusInternalServerError, modifyErr
		}

		if h.StartedBy == evergreen.User && !h.NeedsNewAgentMonitor {
			return http.StatusOK, nil
		}

		// Enqueue the job immediately, if possible.
		if err := units.EnqueueHostReprovisioningJob(ctx, env, h); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message":           "could not enqueue job to reprovision host",
				"host_id":           h.Id,
				"needs_reprovision": h.NeedsReprovision,
			}))
		}

		return http.StatusOK, nil
	}
}

func GetReprovisionToNewCallback(ctx context.Context, env evergreen.Environment, username string) func(h *host.Host) (int, error) {
	return func(h *host.Host) (int, error) {
		modifyErr := h.SetNeedsReprovisionToNew(ctx, username)
		if modifyErr != nil {
			return http.StatusInternalServerError, modifyErr
		}

		if h.StartedBy == evergreen.User && !h.NeedsNewAgentMonitor {
			return http.StatusOK, nil
		}

		// Enqueue the job immediately, if possible.
		if err := units.EnqueueHostReprovisioningJob(ctx, env, h); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message":           "could not enqueue job to reprovision host",
				"host_id":           h.Id,
				"needs_reprovision": h.NeedsReprovision,
			}))
		}

		return http.StatusOK, nil
	}
}

func GetUpdateHostStatusCallback(ctx context.Context, env evergreen.Environment,
	status string, notes string, user *user.DBUser) func(h *host.Host) (httpStatus int, err error) {
	return func(h *host.Host) (httpStatus int, err error) {
		_, httpStatus, modifyErr := ModifyHostStatus(ctx, env, h, status, notes, user)
		return httpStatus, modifyErr
	}
}
