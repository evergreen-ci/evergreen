package data

import (
	"context"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"net/http"
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

func (hc *DBConnector) GetPaginatedRunningHosts(hostID, distroID, currentTaskID string, statuses []string, startedBy string, sortBy string, sortDir, page, limit int) ([]host.Host, *int, int, error) {
	hosts, filteredHostsCount, totalHostsCount, err := host.GetPaginatedRunningHosts(hostID, distroID, currentTaskID, statuses, startedBy, sortBy, sortDir, page, limit)
	if err != nil {
		return nil, nil, 0, err
	}
	return hosts, filteredHostsCount, totalHostsCount, nil
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

func TerminateHost(ctx context.Context, host *host.Host, user string) error {
	return errors.WithStack(cloud.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), host, user, "terminated via REST API"))
}

func CheckHostSecret(hostID string, r *http.Request) (int, error) {
	_, code, err := model.ValidateHost(hostID, r)
	return code, errors.WithStack(err)
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

func FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	hostById, err := host.FindOneId(hostID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
		}
	}
	if hostById == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id '%s' not found", hostID),
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
