package data

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
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

func (hc *DBHostConnector) FindHostsByDistroID(distroID string) ([]host.Host, error) {
	return host.Find(host.ByDistroId(distroID))
}

// NewIntentHost is a method to insert an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *DBHostConnector) NewIntentHost(options *restmodel.HostRequestOptions, user *user.DBUser) (*host.Host, error) {

	// Get key value if PublicKey is a name
	keyVal, err := user.GetPublicKey(options.KeyName)
	if err != nil {
		keyVal = options.KeyName
	}
	if keyVal == "" {
		return nil, errors.New("invalid key")
	}

	var providerSettings *map[string]interface{}
	if options.UserData != "" {
		var d distro.Distro
		d, err = distro.FindOne(distro.ById(options.DistroID))
		if err != nil {
			return nil, errors.Wrapf(err, "error finding distro '%s'", options.DistroID)
		}
		providerSettings = d.ProviderSettings
		(*providerSettings)["user_data"] = options.UserData
	}

	spawnOptions := cloud.SpawnOptions{
		DistroId:         options.DistroID,
		ProviderSettings: providerSettings,
		UserName:         user.Username(),
		PublicKey:        keyVal,
		TaskId:           options.TaskID,
		Owner:            user,
		InstanceTags:     options.InstanceTags,
	}

	intentHost, err := cloud.CreateSpawnHost(spawnOptions)
	if err != nil {
		return nil, err
	}

	if err := intentHost.Insert(); err != nil {
		return nil, err
	}
	return intentHost, nil
}

func (hc *DBHostConnector) SetHostStatus(host *host.Host, status, user string) error {
	return host.SetStatus(status, user, "")
}

func (hc *DBHostConnector) SetHostExpirationTime(host *host.Host, newExp time.Time) error {
	if err := host.SetExpirationTime(newExp); err != nil {
		return errors.Wrap(err, "Error extending host expiration time")
	}

	return nil
}

func (hc *DBHostConnector) TerminateHost(ctx context.Context, host *host.Host, user string) error {
	return errors.WithStack(cloud.TerminateSpawnHost(ctx, host, evergreen.GetEnvironment().Settings(), user))
}

func (hc *DBHostConnector) CheckHostSecret(r *http.Request) (int, error) {
	_, code, err := model.ValidateHost("", r)
	return code, errors.WithStack(err)
}

// MockHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with the backing database.
type MockHostConnector struct {
	CachedHosts []host.Host
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

func (hc *MockHostConnector) FindHostsByDistroID(distroID string) ([]host.Host, error) {
	hosts := []host.Host{}
	for _, h := range hc.CachedHosts {
		if h.Distro.Id == distroID {
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}

// NewIntentHost is a method to mock "insert" an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *MockHostConnector) NewIntentHost(options *restmodel.HostRequestOptions, user *user.DBUser) (*host.Host, error) {
	keyVal := strings.Join([]string{"ssh-rsa", base64.StdEncoding.EncodeToString([]byte("foo"))}, " ")

	spawnOptions := cloud.SpawnOptions{
		DistroId:         options.DistroID,
		UserName:         user.Username(),
		ProviderSettings: &map[string]interface{}{"user_data": options.UserData, "ami": "ami-123456"},
		PublicKey:        keyVal,
		TaskId:           options.TaskID,
		Owner:            user,
		InstanceTags:     options.InstanceTags,
	}

	intentHost, err := cloud.CreateSpawnHost(spawnOptions)
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

func (hc *MockHostConnector) CheckHostSecret(r *http.Request) (int, error) {
	if r.Header.Get(evergreen.HostSecretHeader) == "" {
		return http.StatusBadRequest, errors.New("Bad request")
	}
	return http.StatusOK, nil
}

func (dbc *MockConnector) FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	return findHostByIdWithOwner(dbc, hostID, user)
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
		if !auth.IsSuperUser(c.GetSuperUsers(), user) {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized to modify host",
			}
		}
	}

	return host, nil
}
