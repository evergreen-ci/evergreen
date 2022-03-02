package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

var errHostStatusChangeConflict = errors.New("conflicting host status modification is in progress")

// To be moved to a better home when we restructure the resolvers.go file
// TerminateSpawnHost is a shared utility function to terminate a spawn host
func TerminateSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if h.Status == evergreen.HostTerminated {
		err := errors.New(fmt.Sprintf("Host %v is already terminated", h.Id))
		return http.StatusBadRequest, err
	}

	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	terminateJob := units.NewSpawnHostTerminationJob(h, u.Id, ts)
	if err := env.RemoteQueue().Put(ctx, terminateJob); err != nil {
		if amboy.IsDuplicateJobScopeError(err) {
			err = errHostStatusChangeConflict
		}
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

// StopSpawnHost is a shared utility function to Stop a running spawn host
func StopSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if h.Status == evergreen.HostStopped || h.Status == evergreen.HostStopping {
		err := errors.New(fmt.Sprintf("Host %v is already stopping or stopped", h.Id))
		return http.StatusBadRequest, err

	}
	if h.Status != evergreen.HostRunning {
		err := errors.New(fmt.Sprintf("Host %v is not running", h.Id))
		return http.StatusBadRequest, err
	}

	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	stopJob := units.NewSpawnhostStopJob(h, u.Id, ts)
	if err := env.RemoteQueue().Put(ctx, stopJob); err != nil {
		if amboy.IsDuplicateJobScopeError(err) {
			err = errHostStatusChangeConflict
		}
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil

}

// StartSpawnHost is a shared utility function to Start a stopped spawn host
func StartSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if h.Status == evergreen.HostStarting || h.Status == evergreen.HostRunning {
		err := errors.New(fmt.Sprintf("Host %v is already starting or running", h.Id))
		return http.StatusBadRequest, err

	}

	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	startJob := units.NewSpawnhostStartJob(h, u.Id, ts)
	if err := env.RemoteQueue().Put(ctx, startJob); err != nil {
		if amboy.IsDuplicateJobScopeError(err) {
			err = errHostStatusChangeConflict
		}

		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}
