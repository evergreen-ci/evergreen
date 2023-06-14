package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func FindHostsInRange(apiParams restmodel.APIHostParams, username string) ([]host.Host, error) {
	params := host.HostsInRangeParams{
		CreatedBefore: apiParams.CreatedBefore,
		CreatedAfter:  apiParams.CreatedAfter,
		Distro:        apiParams.Distro,
		UserSpawned:   apiParams.UserSpawned,
		Status:        apiParams.Status,
		Region:        apiParams.Region,
		User:          username,
	}

	hostRes, err := host.FindHostsInRange(params)
	if err != nil {
		return nil, err
	}

	return hostRes, nil
}

// NewIntentHost is a method to insert an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func NewIntentHost(ctx context.Context, options *restmodel.HostRequestOptions, user *user.DBUser,
	env evergreen.Environment) (*host.Host, error) {
	spawnOptions, err := makeSpawnOptions(options, user)
	if err != nil {
		return nil, err
	}

	intentHost, err := cloud.CreateSpawnHost(ctx, *spawnOptions, env.Settings())
	if err != nil {
		return nil, errors.Wrap(err, "creating spawn host")
	}

	if err := intentHost.Insert(); err != nil {
		return nil, err
	}

	grip.Info(message.Fields{
		"message":  "inserted intent host",
		"host_tag": intentHost.Tag,
		"distro":   intentHost.Distro.Id,
		"user":     user.Username(),
	})

	if err := units.EnqueueHostCreateJobs(ctx, env, []host.Host{*intentHost}); err != nil {
		return nil, errors.Wrapf(err, "enqueueing host create job for '%s'", intentHost.Id)
	}

	return intentHost, nil
}

// GenerateHostProvisioningScript generates and returns the script to
// provision the host given by host ID.
func GenerateHostProvisioningScript(ctx context.Context, env evergreen.Environment, hostID string) (string, error) {
	if hostID == "" {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "cannot generate host provisioning script without a host ID",
		}
	}
	h, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding host '%s'", hostID).Error(),
		}
	}
	if h == nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id '%s' not found", hostID),
		}
	}

	creds, err := h.GenerateJasperCredentials(ctx, env)
	if err != nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "generating Jasper credentials").Error(),
		}
	}
	script, err := h.GenerateUserDataProvisioningScript(env.Settings(), creds)
	if err != nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "generating host provisioning script").Error(),
		}
	}
	if err := h.SaveJasperCredentials(ctx, env, creds); err != nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "saving Jasper credentials").Error(),
		}
	}
	return script, nil
}

// FindHostByIdWithOwner finds a host with given host ID that was
// started by the given user. If the given user is a super-user,
// the host will also be returned regardless of who the host was
// started by
func FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	hostById, err := host.FindOneId(hostID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "fetching host information",
		}
	}
	if hostById == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", hostID),
		}
	}

	if user.Username() != hostById.StartedBy {
		if !user.HasPermission(gimlet.PermissionOpts{
			Resource:      hostById.Distro.Id,
			ResourceType:  evergreen.DistroResourceType,
			Permission:    evergreen.PermissionHosts,
			RequiredLevel: evergreen.HostsEdit.Value,
		}) {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized to modify host",
			}
		}
	}

	return hostById, nil
}

var errHostStatusChangeConflict = errors.New("conflicting host status modification is in progress")

// TerminateSpawnHost enqueues a job to terminate a spawn host.
func TerminateSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if h.Status == evergreen.HostTerminated {
		return http.StatusBadRequest, errors.Errorf("host '%s' is already terminated", h.Id)
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

// StopSpawnHost enqueues a job to stop a running spawn host.
func StopSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if h.Status == evergreen.HostStopped {
		return http.StatusBadRequest, errors.Errorf("host '%s' is already stopped", h.Id)
	}
	if h.Status != evergreen.HostRunning && h.Status != evergreen.HostStopping {
		return http.StatusBadRequest, errors.Errorf("host '%s' cannot stop when its status is '%s'", h.Id, h.Status)
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

// StartSpawnHost enqueues a job to start a stopped spawn host.
func StartSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if h.Status != evergreen.HostStopped {
		return http.StatusBadRequest, errors.Errorf("host '%s' cannot be started when its status is '%s'", h.Id, h.Status)
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

// makeSpawnOptions is a utility for validating and converting a HostRequestOptions
// struct into a SpawnOptions struct.
func makeSpawnOptions(options *restmodel.HostRequestOptions, user *user.DBUser) (*cloud.SpawnOptions, error) {
	// Get key value if PublicKey is a name
	keyVal, err := user.GetPublicKey(options.KeyName)
	if err != nil {
		// if the keyname is populated but isn't a valid name, it may be the key value itself
		if options.KeyName == "" {
			return nil, errors.Wrap(err, "key name is empty")
		}
		keyVal = options.KeyName
	}
	if keyVal == "" {
		return nil, errors.Errorf("public key '%s' cannot have an empty value", options.KeyName)
	}
	spawnOptions := cloud.SpawnOptions{
		DistroId:              options.DistroID,
		Userdata:              options.UserData,
		UserName:              user.Username(),
		PublicKey:             keyVal,
		InstanceTags:          options.InstanceTags,
		InstanceType:          options.InstanceType,
		NoExpiration:          options.NoExpiration,
		IsVirtualWorkstation:  options.IsVirtualWorkstation,
		IsCluster:             options.IsCluster,
		HomeVolumeSize:        options.HomeVolumeSize,
		HomeVolumeID:          options.HomeVolumeID,
		Region:                options.Region,
		Expiration:            options.Expiration,
		UseProjectSetupScript: options.UseProjectSetupScript,
		ProvisionOptions: &host.ProvisionOptions{
			TaskId:      options.TaskID,
			TaskSync:    options.TaskSync,
			SetupScript: options.SetupScript,
			OwnerId:     user.Id,
		},
	}
	return &spawnOptions, nil
}
