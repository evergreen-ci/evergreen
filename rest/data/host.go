package data

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// DBHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with the backing database.
type DBHostConnector struct{}

// FindHostsById uses the service layer's host type to query the backing database for
// the hosts.
func (hc *DBHostConnector) FindHostsById(id, status, user string, limit int) ([]host.Host, error) {
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

func (hc *DBHostConnector) FindHostsInRange(apiParams restmodel.APIHostParams, username string) ([]host.Host, error) {
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
func (hc *DBHostConnector) FindHostById(id string) (*host.Host, error) {
	h, err := host.FindOne(host.ById(id))
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id %s not found", id),
		}
	}
	return h, nil
}

func (dbc *DBConnector) FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	return findHostByIdWithOwner(dbc, hostID, user)
}

func (hc *DBHostConnector) FindHostsByDistro(distro string) ([]host.Host, error) {
	return host.Find(db.Query(host.ByDistroIDsOrAliasesRunning(distro)))
}

func (hc *DBConnector) GetPaginatedRunningHosts(hostID, distroID, currentTaskID string, statuses []string, startedBy string, sortBy string, sortDir, page, limit int) ([]host.Host, *int, int, error) {
	hosts, filterdHostsCount, totalHostsCount, err := host.GetPaginatedRunningHosts(hostID, distroID, currentTaskID, statuses, startedBy, sortBy, sortDir, page, limit)
	if err != nil {
		return nil, nil, 0, err
	}
	return hosts, filterdHostsCount, totalHostsCount, nil
}

func (hc *DBConnector) GetHostByIdWithTask(hostID string) (*host.Host, error) {
	host, err := host.GetHostByIdWithTask(hostID)
	if err != nil {
		return nil, err
	}
	return host, nil
}

// NewIntentHost is a method to insert an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *DBHostConnector) NewIntentHost(ctx context.Context, options *restmodel.HostRequestOptions, user *user.DBUser,
	settings *evergreen.Settings) (*host.Host, error) {

	// Get key value if PublicKey is a name
	keyVal, err := user.GetPublicKey(options.KeyName)
	if err != nil {
		keyVal = options.KeyName
	}
	if keyVal == "" {
		return nil, errors.New("invalid key")
	}

	spawnOptions := cloud.SpawnOptions{
		DistroId:             options.DistroID,
		Userdata:             options.UserData,
		UserName:             user.Username(),
		PublicKey:            keyVal,
		TaskId:               options.TaskID,
		TaskSync:             options.TaskSync,
		Owner:                user,
		InstanceTags:         options.InstanceTags,
		InstanceType:         options.InstanceType,
		NoExpiration:         options.NoExpiration,
		IsVirtualWorkstation: options.IsVirtualWorkstation,
		IsCluster:            options.IsCluster,
		HomeVolumeSize:       options.HomeVolumeSize,
		HomeVolumeID:         options.HomeVolumeID,
		Region:               options.Region,
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

func (hc *DBHostConnector) SetHostStatus(host *host.Host, status, user string) error {
	return host.SetStatus(status, user, fmt.Sprintf("changed by %s from API", user))
}

func (hc *DBHostConnector) SetHostExpirationTime(host *host.Host, newExp time.Time) error {
	if err := host.SetExpirationTime(newExp); err != nil {
		return errors.Wrap(err, "Error extending host expiration time")
	}

	return nil
}

func (hc *DBHostConnector) TerminateHost(ctx context.Context, host *host.Host, user string) error {
	return errors.WithStack(cloud.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), host, user, "terminated via REST API"))
}

func (hc *DBHostConnector) CheckHostSecret(hostID string, r *http.Request) (int, error) {
	_, code, err := model.ValidateHost(hostID, r)
	return code, errors.WithStack(err)
}

func (hc *DBHostConnector) FindVolumeById(volumeID string) (*host.Volume, error) {
	return host.FindVolumeByID(volumeID)
}

func (hc *DBHostConnector) FindVolumesByUser(user string) ([]host.Volume, error) {
	return host.FindVolumesByUser(user)
}

func (hc *DBHostConnector) SetVolumeName(v *host.Volume, name string) error {
	return v.SetDisplayName(name)
}

func (hc *DBHostConnector) FindHostWithVolume(volumeID string) (*host.Host, error) {
	return host.FindHostWithVolume(volumeID)
}

func (hc *DBHostConnector) AggregateSpawnhostData() (*host.SpawnHostUsage, error) {
	data, err := host.AggregateSpawnhostData()
	if err != nil {
		return nil, errors.Wrap(err, "error getting spawn host data")
	}
	return data, nil
}

// MockHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with the backing database.
type MockHostConnector struct {
	CachedHosts   []host.Host
	CachedVolumes []host.Volume
}

// FindHostsById searches the mock hosts slice for hosts and returns them
func (hc *MockHostConnector) FindHostsById(id, status, user string, limit int) ([]host.Host, error) {
	if id != "" && user == "" && status == "" {
		return hc.FindHostsByIdOnly(id, status, user, limit)
	}

	var hostsToReturn []host.Host
	for ix := range hc.CachedHosts {
		h := hc.CachedHosts[ix]
		if id != "" {
			continue
		}
		if user != "" && h.StartedBy != user {
			continue
		}
		if status != "" {
			if h.Status != status {
				continue
			}
		} else {
			statusFound := false
			for _, status := range evergreen.UpHostStatus {
				if h.Status == status {
					statusFound = true
				}
			}
			if !statusFound {
				continue
			}
		}

		hostsToReturn = append(hostsToReturn, h)
		if len(hostsToReturn) >= limit {
			return hostsToReturn, nil
		}
	}
	return hostsToReturn, nil
}

// FindHostsInRange searches the mock hosts slice for hosts and returns them
func (hc *MockHostConnector) FindHostsInRange(params restmodel.APIHostParams, username string) ([]host.Host, error) {
	var hostsToReturn []host.Host
	for ix := range hc.CachedHosts {
		h := hc.CachedHosts[ix]
		if username != "" && h.StartedBy != username {
			continue
		}
		if params.Status != "" {
			if h.Status != params.Status {
				continue
			}
		} else {
			if !utility.StringSliceContains(evergreen.UpHostStatus, h.Status) {
				continue
			}
		}

		if params.Distro != "" && h.Distro.Id != params.Distro {
			continue
		}

		if params.Region != "" {
			if len(h.Distro.ProviderSettingsList) < 1 {
				continue
			}
			region, ok := h.Distro.ProviderSettingsList[0].Lookup("region").StringValueOK()
			if !ok || region != params.Region {
				continue
			}
		}

		if params.UserSpawned && !h.UserHost {
			continue
		}

		if h.CreationTime.Before(params.CreatedAfter) {
			continue
		}

		if !utility.IsZeroTime(params.CreatedBefore) && h.CreationTime.After(params.CreatedBefore) {
			continue
		}

		hostsToReturn = append(hostsToReturn, h)
	}
	return hostsToReturn, nil
}

func (hc *MockHostConnector) FindHostsByIdOnly(id, status, user string, limit int) ([]host.Host, error) {
	for ix, h := range hc.CachedHosts {
		if h.Id == id {
			// We've found the host
			var hostsToReturn []host.Host
			if ix+limit > len(hc.CachedHosts) {
				hostsToReturn = hc.CachedHosts[ix:]
			} else {
				hostsToReturn = hc.CachedHosts[ix : ix+limit]
			}
			return hostsToReturn, nil
		}
	}
	return nil, nil
}

func (hc *MockHostConnector) FindHostById(id string) (*host.Host, error) {
	for _, h := range hc.CachedHosts {
		if h.Id == id {
			return &h, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("host with id %s not found", id),
	}
}

func (hc *MockHostConnector) FindHostsByDistro(distro string) ([]host.Host, error) {
	hosts := []host.Host{}
	for _, h := range hc.CachedHosts {
		if h.Status == evergreen.HostRunning && (h.Distro.Id == distro || utility.StringSliceContains(h.Distro.Aliases, distro)) {
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}

// NewIntentHost is a method to mock "insert" an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *MockHostConnector) NewIntentHost(ctx context.Context, options *restmodel.HostRequestOptions, user *user.DBUser, settings *evergreen.Settings) (*host.Host, error) {
	keyVal := strings.Join([]string{"ssh-rsa", base64.StdEncoding.EncodeToString([]byte("foo"))}, " ")

	spawnOptions := cloud.SpawnOptions{
		DistroId:     options.DistroID,
		Userdata:     options.UserData,
		UserName:     user.Username(),
		PublicKey:    keyVal,
		TaskId:       options.TaskID,
		Owner:        user,
		InstanceTags: options.InstanceTags,
		InstanceType: options.InstanceType,
	}

	intentHost, err := cloud.CreateSpawnHost(ctx, spawnOptions, settings)
	if err != nil {
		return nil, err
	}

	hc.CachedHosts = append(hc.CachedHosts, *intentHost)

	return intentHost, nil
}

func (hc *MockHostConnector) SetHostStatus(host *host.Host, status, user string) error {
	for i, _ := range hc.CachedHosts {
		if hc.CachedHosts[i].Id == host.Id {
			hc.CachedHosts[i].Status = status
			host.Status = status
			return nil
		}
	}

	return errors.New("can't find host")
}

func (hc *MockHostConnector) SetHostExpirationTime(host *host.Host, newExp time.Time) error {
	for i, h := range hc.CachedHosts {
		if h.Id == host.Id {
			hc.CachedHosts[i].ExpirationTime = newExp
			host.ExpirationTime = newExp
			return nil
		}
	}

	return errors.New("can't find host")
}

func (hc *MockHostConnector) TerminateHost(ctx context.Context, host *host.Host, user string) error {
	for _, h := range hc.CachedHosts {
		if h.Id == host.Id {
			return nil
		}
	}

	return errors.New("can't find host")
}

func (hc *MockHostConnector) CheckHostSecret(hostID string, r *http.Request) (int, error) {
	if hostID != "" {
		return http.StatusOK, nil
	}
	if r.Header.Get(evergreen.HostSecretHeader) == "" {
		return http.StatusBadRequest, errors.New("Bad request")
	}
	return http.StatusOK, nil
}

func (dbc *MockConnector) FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	return findHostByIdWithOwner(dbc, hostID, user)
}

func (hc *MockConnector) FindVolumeById(volumeID string) (*host.Volume, error) {
	for _, v := range hc.CachedVolumes {
		if v.ID == volumeID {
			return &v, nil
		}
	}
	return nil, nil
}

func (hc *MockConnector) FindVolumesByUser(user string) ([]host.Volume, error) {
	vols := []host.Volume{}
	for _, v := range hc.CachedVolumes {
		if v.CreatedBy == user {
			vols = append(vols, v)
		}
	}
	return vols, nil
}

func (hc *MockConnector) FindHostWithVolume(volumeID string) (*host.Host, error) {
	for _, h := range hc.CachedHosts {
		for _, v := range h.Volumes {
			if v.VolumeID == volumeID {
				return &h, nil
			}
		}
	}
	return nil, nil
}

func (hc *MockConnector) SetVolumeName(volume *host.Volume, name string) error {
	for i := range hc.CachedVolumes {
		if hc.CachedVolumes[i].ID == volume.ID {
			hc.CachedVolumes[i].DisplayName = name
		}
	}
	return nil
}

func (hc *MockConnector) GetPaginatedRunningHosts(hostID, distroID, currentTaskID string, statuses []string, startedBy string, sortBy string, sortDir, page, limit int) ([]host.Host, *int, int, error) {
	return nil, nil, 0, nil
}

func (hc *MockConnector) GetHostByIdWithTask(hostID string) (*host.Host, error) {
	return nil, nil
}

func (hc *MockConnector) AggregateSpawnhostData() (*host.SpawnHostUsage, error) {
	data := host.SpawnHostUsage{}
	usersWithHosts := map[string]bool{} // set for existing users
	data.InstanceTypes = map[string]int{}
	totalComputeCost := 0.0
	for _, h := range hc.CachedHosts {
		if !h.UserHost {
			continue
		}
		data.TotalHosts += 1
		if h.Status == evergreen.HostStopped {
			data.TotalStoppedHosts += 1
		}
		if h.NoExpiration {
			data.TotalUnexpirableHosts += 1
		}
		data.InstanceTypes[h.InstanceType] += 1
		usersWithHosts[h.StartedBy] = true
		totalComputeCost += h.ComputeCostPerHour
	}
	data.AverageComputeCostPerHour = totalComputeCost / float64(data.TotalHosts)
	data.NumUsersWithHosts = len(usersWithHosts)

	usersWithVolumes := map[string]bool{}
	for _, v := range hc.CachedVolumes {
		data.TotalVolumes += 1
		data.TotalVolumeSize += v.Size
		usersWithVolumes[v.CreatedBy] = true
	}
	data.NumUsersWithVolumes = len(usersWithVolumes)
	if data.TotalVolumes == 0 && data.TotalHosts == 0 {
		return nil, errors.New("no host/volume results found")
	}
	return &data, nil
}

func findHostByIdWithOwner(c Connector, hostID string, user gimlet.User) (*host.Host, error) {
	host, err := c.FindHostById(hostID)
	if host == nil {
		return nil, err
	}

	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
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
