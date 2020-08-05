package api

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	adb "github.com/mongodb/anser/db"
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

func GetHostsAndUserPermissions(user *user.DBUser, hostIds []string) ([]host.Host, map[string]gimlet.Permissions, int, error) {
	if len(hostIds) == 0 {
		return nil, nil, http.StatusBadRequest, errors.Errorf("hostIds cannot be empty")
	}

	hosts, err := host.Find(host.ByIds(hostIds))
	if err != nil {
		return nil, nil, http.StatusInternalServerError, errors.Errorf("Error getting hosts to update")
	}
	if len(hosts) == 0 {
		return nil, nil, http.StatusNotFound, errors.Errorf("No matching hosts found")
	}

	var permissions map[string]gimlet.Permissions

	rm := evergreen.GetEnvironment().RoleManager()

	permissions, err = rolemanager.HighestPermissionsForRolesAndResourceType(user.Roles(), evergreen.DistroResourceType, rm)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, errors.Errorf("unable to get user permissions")
	}

	return hosts, permissions, 0, nil
}

// ModifyHostsWithPermissions performs an update on each of the given hosts
// for which the permissions allow updates on that host.
func ModifyHostsWithPermissions(hosts []host.Host, perm map[string]gimlet.Permissions, modifyHost func(h *host.Host) (int, error)) (updated int, httpStatus int, err error) {
	catcher := grip.NewBasicCatcher()

	httpStatus = 0

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

func ModifyHostStatus(queue amboy.Queue, h *host.Host, newStatus string, notes string, u *user.DBUser) (string, int, error) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	currentStatus := h.Status

	if !utility.StringSliceContains(validUpdateToStatuses, newStatus) {
		return "", http.StatusBadRequest, errors.Errorf(InvalidStatusError, newStatus)
	}

	if h.Provider == evergreen.ProviderNameStatic && newStatus == evergreen.HostDecommissioned {
		return "", http.StatusBadRequest, errors.Errorf(DecommissionStaticHostError)
	}

	if newStatus == evergreen.HostTerminated {
		if !queue.Info().Started {
			return "", http.StatusInternalServerError, errors.Errorf(HostTerminationQueueingError)
		}

		if err := queue.Put(ctx, units.NewHostTerminationJob(env, h, true, fmt.Sprintf("terminated by %s", u.Username()))); err != nil {
			return "", http.StatusInternalServerError, errors.Errorf(HostTerminationQueueingError)
		}
		return fmt.Sprintf(HostTerminationQueueingSuccess, h.Id), 0, nil
	}

	err := h.SetStatus(newStatus, u.Id, notes)
	if err != nil {
		return "", http.StatusInternalServerError, errors.Errorf(HostUpdateError, err.Error)
	}

	return fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status), 0, nil
}

func GetRestartJasperCallback(username string) func(h *host.Host) (int, error) {
	return func(h *host.Host) (int, error) {
		modifyErr := h.SetNeedsJasperRestart(username)
		if adb.ResultsNotFound(modifyErr) {
			return 0, nil
		}
		if modifyErr != nil {
			return http.StatusInternalServerError, modifyErr
		}
		return 0, nil
	}
}

func GetUpdateHostStatusCallback(rq amboy.Queue, status string, notes string, user *user.DBUser) func(h *host.Host) (httpStatus int, err error) {
	return func(h *host.Host) (httpStatus int, err error) {
		_, httpStatus, modifyErr := ModifyHostStatus(rq, h, status, notes, user)
		return httpStatus, modifyErr
	}
}
