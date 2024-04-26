package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func FindHostsInRange(ctx context.Context, apiParams restmodel.APIHostParams, username string) ([]host.Host, error) {
	params := host.HostsInRangeParams{
		CreatedBefore: apiParams.CreatedBefore,
		CreatedAfter:  apiParams.CreatedAfter,
		Distro:        apiParams.Distro,
		UserSpawned:   apiParams.UserSpawned,
		Status:        apiParams.Status,
		Region:        apiParams.Region,
		User:          username,
	}

	hostRes, err := host.FindHostsInRange(ctx, params)
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

	if err := intentHost.Insert(ctx); err != nil {
		return nil, err
	}
	event.LogHostCreated(intentHost.Id)
	grip.Info(message.Fields{
		"message":  "inserted intent host",
		"host_id":  intentHost.Id,
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
	h, err := host.FindOneByIdOrTag(ctx, hostID)
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
	script, err := h.GenerateUserDataProvisioningScript(ctx, env.Settings(), creds)
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
func FindHostByIdWithOwner(ctx context.Context, hostID string, user gimlet.User) (*host.Host, error) {
	hostById, err := host.FindOneId(ctx, hostID)
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
	if err := units.EnqueueSpawnHostModificationJob(ctx, env, terminateJob); err != nil {
		if amboy.IsDuplicateJobScopeError(err) {
			err = errHostStatusChangeConflict
		}
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

// StopSpawnHost enqueues a job to stop a running spawn host.
func StopSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host, shouldKeepOff bool) (int, error) {
	if !utility.StringSliceContains(evergreen.StoppableHostStatuses, h.Status) {
		return http.StatusBadRequest, errors.Errorf("host '%s' cannot be stopped because because its status ('%s') is not a stoppable state", h.Id, h.Status)
	}

	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	stopJob := units.NewSpawnhostStopJob(h, shouldKeepOff, evergreen.ModifySpawnHostManual, u.Id, ts)
	if err := units.EnqueueSpawnHostModificationJob(ctx, env, stopJob); err != nil {
		if amboy.IsDuplicateJobScopeError(err) {
			err = errHostStatusChangeConflict
		}
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil

}

// StartSpawnHost enqueues a job to start a stopped spawn host.
func StartSpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host) (int, error) {
	if !utility.StringSliceContains(evergreen.StartableHostStatuses, h.Status) {
		return http.StatusBadRequest, errors.Errorf("host '%s' cannot be started because because its status ('%s') is not a startable state", h.Id, h.Status)
	}

	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	startJob := units.NewSpawnhostStartJob(h, evergreen.ModifySpawnHostManual, u.Id, ts)
	if err := units.EnqueueSpawnHostModificationJob(ctx, env, startJob); err != nil {
		if amboy.IsDuplicateJobScopeError(err) {
			err = errHostStatusChangeConflict
		}
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

// StartSpawnHost enqueues a job to modify a spawn host.
func ModifySpawnHost(ctx context.Context, env evergreen.Environment, u *user.DBUser, h *host.Host, opts host.HostModifyOptions) (int, error) {
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	modifyJob := units.NewSpawnhostModifyJob(h, opts, ts)
	if err := units.EnqueueSpawnHostModificationJob(ctx, env, modifyJob); err != nil {
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

// PostHostIsUp indicates to the app server that a host is up.
func PostHostIsUp(ctx context.Context, params restmodel.APIHostIsUpOptions) (*restmodel.APIHost, error) {
	h, err := host.FindOneByIdOrTag(ctx, params.HostID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding host '%s'", params.HostID).Error(),
		}
	}
	if h == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", params.HostID),
		}
	}

	if err := fixProvisioningIntentHost(ctx, h, params.EC2InstanceID); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "fixing intent host").Error(),
		}
	}

	var apiHost model.APIHost
	apiHost.BuildFromService(h, nil)
	return &apiHost, nil
}

func fixProvisioningIntentHost(ctx context.Context, h *host.Host, instanceID string) error {
	if cloud.IsEC2InstanceID(h.Id) {
		// If the host already has an instance ID, it's not an intent host.
		return nil
	}
	if !evergreen.IsEc2Provider(h.Distro.Provider) {
		// Intent host issues only affect ephemeral (i.e. EC2) hosts.
		return nil
	}
	if instanceID == "" {
		// If the host is an intent host but the agent does not send the EC2
		// instance ID, there's nothing that can be done to fix it here.

		// TODO (DEVPROD-6752): should log and return an error once all hosts
		// roll over from the deploy. All intent hosts should be sending their
		// EC2 instance ID. If they don't, it should fail provisioning and
		// should not start the agent.
		// msg := "intent host is running, but it did not provide an EC2 instance ID, which is required"
		// grip.Warning(message.Fields{
		//     "message":     msg,
		//     "host_id":     h.Id,
		//     "host_status": h.Status,
		//     "provider":    h.Distro.Provider,
		//     "distro":      h.Distro.Id,
		// })
		return nil
	}

	env := evergreen.GetEnvironment()
	switch h.Status {
	case evergreen.HostBuilding:
		return errors.Wrap(transitionIntentHostToStarting(ctx, env, h, instanceID), "starting intent host that actually succeeded")
	case evergreen.HostBuildingFailed, evergreen.HostTerminated:
		return errors.Wrap(transitionIntentHostToDecommissioned(ctx, env, h, instanceID), "decommissioning intent host")
	default:
		return errors.Errorf("logical error: intent host is in state '%s', which should be impossible when the agent is running", h.Status)
	}
}

func transitionIntentHostToStarting(ctx context.Context, env evergreen.Environment, hostToStart *host.Host, instanceID string) error {
	grip.Notice(message.Fields{
		"message":     "DB-EC2 state mismatch - EC2 instance started but Evergreen still has it stored as an intent host, fixing now",
		"old_host_id": hostToStart.Id,
		"new_host_id": instanceID,
		"host_tag":    hostToStart.Tag,
		"distro":      hostToStart.Distro.Id,
		"host_status": hostToStart.Status,
	})

	intentHostID := hostToStart.Id
	hostToStart.Id = instanceID
	hostToStart.Status = evergreen.HostStarting
	hostToStart.StartTime = time.Now()
	if err := host.UnsafeReplace(ctx, env, intentHostID, hostToStart); err != nil {
		return errors.Wrap(err, "replacing intent host with real host")
	}

	event.LogHostStartSucceeded(hostToStart.Id)

	return nil
}

func transitionIntentHostToDecommissioned(ctx context.Context, env evergreen.Environment, hostToDecommission *host.Host, instanceID string) error {
	grip.Notice(message.Fields{
		"message":     "DB-EC2 state mismatch - EC2 instance started but Evergreen already gave up on this host, fixing now",
		"host_id":     hostToDecommission.Id,
		"instance_id": instanceID,
		"host_status": hostToDecommission.Status,
	})

	intentHostID := hostToDecommission.Id
	hostToDecommission.Id = instanceID
	oldStatus := hostToDecommission.Status
	hostToDecommission.Status = evergreen.HostDecommissioned
	if err := host.UnsafeReplace(ctx, env, intentHostID, hostToDecommission); err != nil {
		return errors.Wrap(err, "replacing intent host with real host")
	}

	event.LogHostStatusChanged(hostToDecommission.Id, oldStatus, hostToDecommission.Status, evergreen.User, "host started agent but intent host is already considered a failure")
	grip.Info(message.Fields{
		"message":    "intent host decommissioned",
		"host_id":    hostToDecommission.Id,
		"host_tag":   hostToDecommission.Tag,
		"distro":     hostToDecommission.Distro.Id,
		"old_status": oldStatus,
	})

	return nil
}
