package data

import (
	"errors"
	"net/http"

	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/spawn"
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
		return nil, &rest.APIError{
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
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id %s not found", id),
		}
	}
	return h, nil
}

// NewIntentHost is a method to insert an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *DBHostConnector) NewIntentHost(distroID string, keyNameOrVal string, user *user.DBUser) (*host.Host, error) {
	keyVal, err := user.GetPublicKey(keyNameOrVal)
	if err != nil {
		keyVal = keyNameOrVal
	}
	if keyVal == "" {
		return nil, errors.New("invalid key")
	}

	spawnOptions := spawn.Options{
		Distro:    distroID,
		UserName:  user.Username(),
		PublicKey: keyVal,
	}

	if err = spawn.Validate(spawnOptions); err != nil {
		return nil, err
	}

	intentHost, err := spawn.CreateHost(spawnOptions, user)
	if err != nil {
		return nil, err
	}

	if err := intentHost.Insert(); err != nil {
		return nil, err
	}

	return intentHost, nil
}

// MockHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with he backing database.
type MockHostConnector struct {
	CachedHosts []host.Host
}

// FindHostsById searches the mock hosts slice for hosts and returns them
func (hc *MockHostConnector) FindHostsById(id, status, user string, limit int, sort int) ([]host.Host, error) {
	var hostsToReturn []host.Host
	for ix := range hc.CachedHosts {
		var h host.Host
		if sort < 1 {
			h = hc.CachedHosts[len(hc.CachedHosts)-1-ix]
		} else {
			h = hc.CachedHosts[ix]
		}

		if id != "" {
			if (sort < 1 && h.Id > id) || (sort > 1 && h.Id < id) {
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

func (hc *MockHostConnector) FindHostById(id string) (*host.Host, error) {
	for _, h := range hc.CachedHosts {
		if h.Id == id {
			return &h, nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("host with id %s not found", id),
	}
}

// NewIntentHost is a method to mock "insert" an intent host given a distro and a public key
// The public key can be the name of a saved key or the actual key string
func (hc *MockHostConnector) NewIntentHost(distroID string, keyNameOrVal string, user *user.DBUser) (*host.Host, error) {
	keyVal, err := user.GetPublicKey(keyNameOrVal)
	if err != nil {
		keyVal = keyNameOrVal
	}
	if keyVal == "" {
		return nil, errors.New("invalid key")
	}

	spawnOptions := spawn.Options{
		Distro:    distroID,
		UserName:  user.Username(),
		PublicKey: keyVal,
	}

	if err = spawn.Validate(spawnOptions); err != nil {
		return nil, err
	}

	intentHost, err := spawn.CreateHost(spawnOptions, user)
	if err != nil {
		return nil, err
	}

	hc.CachedHosts = append(hc.CachedHosts, *intentHost)

	return intentHost, nil
}
