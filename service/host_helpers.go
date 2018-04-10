package service

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
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
	UnrecognizedAction             = "Unrecognized action: %v"
)

func modifyHostStatus(h *host.Host, opts *uiParams, u *user.DBUser) (string, error) {
	env := evergreen.GetEnvironment()
	queue := env.RemoteQueue()

	switch opts.Action {
	case "updateStatus":
		currentStatus := h.Status
		newStatus := opts.Status
		if !util.StringSliceContains(validUpdateToStatuses, newStatus) {
			return "", errors.Errorf(InvalidStatusError, newStatus)
		}

		if h.Provider == evergreen.ProviderNameStatic && newStatus == evergreen.HostDecommissioned {
			return "", errors.New(DecommissionStaticHostError)
		}

		if newStatus == evergreen.HostTerminated {
			if err := queue.Put(units.NewHostTerminationJob(env, *h)); err != nil {
				return "", errors.New(HostTerminationQueueingError)
			}
			return fmt.Sprintf(HostTerminationQueueingSuccess, h.Id), nil
		}

		err := h.SetStatus(newStatus, u.Id, "")
		if err != nil {
			return "", errors.Wrap(err, HostUpdateError)
		}
		return fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status), nil
	default:
		return "", errors.Errorf(UnrecognizedAction, opts.Action)
	}
}
