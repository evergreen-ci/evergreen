package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// FindHostsById uses the service layer's host type to query the backing database for
// the hosts.
func FindHostsById(id, status, user string, limit int) ([]host.Host, error) {
	hostRes, err := host.GetHostsByFromIDWithStatus(id, status, user, limit)
	if err != nil {
		return nil, err
	}
	if len(hostRes) == 0 {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no hosts found",
		}
	}
	return hostRes, nil
}

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

// FindHostById queries the database for the host with id matching the hostId
func FindHostById(id string) (*host.Host, error) {
	h, err := host.FindOne(host.ById(id))
	if err != nil {
		return nil, err
	}

	return h, nil
}

// FindHostByIP queries the database for the host with ip matching the ip address
func FindHostByIpAddress(ip string) (*host.Host, error) {
	h, err := host.FindOne(host.ByIP(ip))
	if err != nil {
		return nil, err
	}

	return h, nil
}

func FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	return findHostByIdWithOwner(hostID, user)
}

func FindHostsByDistro(distro string) ([]host.Host, error) {
	return host.Find(db.Query(host.ByDistroIDsOrAliasesRunning(distro)))
}

func (hc *DBConnector) GetPaginatedRunningHosts(hostID, distroID, currentTaskID string, statuses []string, startedBy string, sortBy string, sortDir, page, limit int) ([]host.Host, *int, int, error) {
	hosts, filteredHostsCount, totalHostsCount, err := host.GetPaginatedRunningHosts(hostID, distroID, currentTaskID, statuses, startedBy, sortBy, sortDir, page, limit)
	if err != nil {
		return nil, nil, 0, err
	}
	return hosts, filteredHostsCount, totalHostsCount, nil
}

func GetHostByIdOrTagWithTask(hostID string) (*host.Host, error) {
	host, err := host.GetHostByIdOrTagWithTask(hostID)
	if err != nil {
		return nil, err
	}
	return host, nil
}

// NewIntentHost is a method to insert an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func NewIntentHost(ctx context.Context, options *restmodel.HostRequestOptions, user *user.DBUser,
	settings *evergreen.Settings) (*host.Host, error) {

	// Get key value if PublicKey is a name
	keyVal, err := user.GetPublicKey(options.KeyName)
	if err != nil {
		// if the keyname is populated but isn't a valid name, it may be the key value itself
		if options.KeyName == "" {
			return nil, err
		}
		keyVal = options.KeyName
	}
	if keyVal == "" {
		return nil, errors.Errorf("the value for key name '%s' is empty", options.KeyName)
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
	intentHost, err := cloud.CreateSpawnHost(ctx, spawnOptions, settings)
	if err != nil {
		return nil, errors.Wrap(err, "error creating spawn host")
	}

	if err := intentHost.Insert(); err != nil {
		return nil, err
	}
	return intentHost, nil
}

func SetHostStatus(host *host.Host, status, user string) error {
	return host.SetStatus(status, user, fmt.Sprintf("changed by %s from API", user))
}

func SetHostExpirationTime(host *host.Host, newExp time.Time) error {
	if err := host.SetExpirationTime(newExp); err != nil {
		return errors.Wrap(err, "Error extending host expiration time")
	}

	return nil
}

func TerminateHost(ctx context.Context, host *host.Host, user string) error {
	return errors.WithStack(cloud.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), host, user, "terminated via REST API"))
}

// DisableHost disables the host, notifies it's been disabled,
// and clears and resets its running task.
func DisableHost(ctx context.Context, env evergreen.Environment, host *host.Host, reason string) error {
	return units.HandlePoisonedHost(ctx, env, host, reason)
}

func CheckHostSecret(hostID string, r *http.Request) (int, error) {
	_, code, err := model.ValidateHost(hostID, r)
	return code, errors.WithStack(err)
}

func FindVolumeById(volumeID string) (*host.Volume, error) {
	return host.FindVolumeByID(volumeID)
}

func FindVolumesByUser(user string) ([]host.Volume, error) {
	return host.FindVolumesByUser(user)
}

func SetVolumeName(v *host.Volume, name string) error {
	return v.SetDisplayName(name)
}

func FindHostWithVolume(volumeID string) (*host.Host, error) {
	return host.FindHostWithVolume(volumeID)
}

func AggregateSpawnhostData() (*host.SpawnHostUsage, error) {
	data, err := host.AggregateSpawnhostData()
	if err != nil {
		return nil, errors.Wrap(err, "error getting spawn host data")
	}
	return data, nil
}

func GenerateHostProvisioningScript(ctx context.Context, hostID string) (string, error) {
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
			Message:    errors.Wrapf(err, "finding host with ID '%s'", hostID).Error(),
		}
	}
	if h == nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id '%s' not found", hostID),
		}
	}

	env := evergreen.GetEnvironment()
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

func findHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	host, err := FindHostById(hostID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
		}
	}
	if host == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id '%s' not found", hostID),
		}
	}

	if user.Username() != host.StartedBy {
		if !user.HasPermission(gimlet.PermissionOpts{
			Resource:      host.Distro.Id,
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

	return host, nil
}
