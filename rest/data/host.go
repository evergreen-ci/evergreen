package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with the backing database.
type DBHostConnector struct{}

// FindHostsById uses the service layer's host type to query the backing database for
// the hosts.
func (hc *DBHostConnector) FindHostsById(id, status, user string, limit int, sortDir int) ([]host.Host, error) {
	hostRes, err := host.GetHostsByFromIdWithStatus(id, status, user, limit, sortDir)
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

// NewIntentHost is a method to insert an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *DBHostConnector) NewIntentHost(distroID, keyNameOrVal, taskID string, user *user.DBUser) (*host.Host, error) {
	keyVal, err := user.GetPublicKey(keyNameOrVal)
	if err != nil {
		keyVal = keyNameOrVal
	}
	if keyVal == "" {
		return nil, errors.New("invalid key")
	}

	spawnOptions := cloud.SpawnOptions{
		Distro:    distroID,
		UserName:  user.Username(),
		PublicKey: keyVal,
		TaskId:    taskID,
		Owner:     user,
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

// MockHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with he backing database.
type MockHostConnector struct {
	CachedHosts []host.Host
}

// FindHostsById searches the mock hosts slice for hosts and returns them
func (hc *MockHostConnector) FindHostsById(id, status, user string, limit int, sort int) ([]host.Host, error) {
	if id != "" && user == "" && status == "" {
		return hc.FindHostsByIdOnly(id, status, user, limit, sort)
	}

	var hostsToReturn []host.Host
	for ix := range hc.CachedHosts {
		var h host.Host
		if sort < 0 {
			h = hc.CachedHosts[len(hc.CachedHosts)-1-ix]
		} else {
			h = hc.CachedHosts[ix]
		}
		if id != "" {
			if (sort < 0 && h.Id > id) || (sort > 0 && h.Id < id) {
				continue
			}
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
			for _, status := range evergreen.UphostStatus {
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

func (hc *MockHostConnector) FindHostsByIdOnly(id, status, user string, limit int, sort int) ([]host.Host, error) {
	for ix, h := range hc.CachedHosts {
		if h.Id == id {
			// We've found the host
			var hostsToReturn []host.Host
			if sort < 0 {
				if ix-limit > 0 {
					hostsToReturn = hc.CachedHosts[ix-(limit) : ix]
				} else {
					hostsToReturn = hc.CachedHosts[:ix]
				}
			} else {
				if ix+limit > len(hc.CachedHosts) {
					hostsToReturn = hc.CachedHosts[ix:]
				} else {
					hostsToReturn = hc.CachedHosts[ix : ix+limit]
				}
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

// NewIntentHost is a method to mock "insert" an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *MockHostConnector) NewIntentHost(distroID, keyNameOrVal, taskID string, user *user.DBUser) (*host.Host, error) {
	keyVal, err := user.GetPublicKey(keyNameOrVal)
	if err != nil {
		keyVal = keyNameOrVal
	}
	if keyVal == "" {
		return nil, errors.New("invalid key")
	}

	spawnOptions := cloud.SpawnOptions{
		Distro:    distroID,
		UserName:  user.Username(),
		PublicKey: keyVal,
		TaskId:    taskID,
		Owner:     user,
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

func (dbc *MockConnector) FindHostByIdWithOwner(hostID string, user gimlet.User) (*host.Host, error) {
	return findHostByIdWithOwner(dbc, hostID, user)
}

func findHostByIdWithOwner(c Connector, hostID string, user gimlet.User) (*host.Host, error) {
	host, err := c.FindHostById(hostID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
		}
	}
	if host == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "host does not exist",
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
