package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
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

func modifyHostStatus(queue amboy.Queue, h *host.Host, opts *uiParams, u *user.DBUser) (string, error) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	switch opts.Action {
	case "updateStatus":
		currentStatus := h.Status
		newStatus := opts.Status
		if !util.StringSliceContains(validUpdateToStatuses, newStatus) {
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
			if !queue.Started() {
				return "", gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    HostTerminationQueueingError,
				}
			}

			if err := queue.Put(ctx, units.NewHostTerminationJob(env, *h, true)); err != nil {
				return "", gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    HostTerminationQueueingError,
				}
			}
			return fmt.Sprintf(HostTerminationQueueingSuccess, h.Id), nil
		}

		err := h.SetStatus(newStatus, u.Id, opts.Notes)
		if err != nil {
			return "", gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrap(err, HostUpdateError).Error(),
			}
		}
		return fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status), nil
	default:
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf(UnrecognizedAction, opts.Action),
		}
	}
}
