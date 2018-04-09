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
