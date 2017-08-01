package data

import (
	"errors"
	"net/http"

	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/spawn"
)

// DBHostConnector is a struct that implements the Host related methods
// from the Connector through interactions with the backing database.
type DBHostConnector struct{}

// FindHosts uses the service layer's host type to query the backing database for
// the hosts.
func (hc *DBHostConnector) FindHostsById(id, status string, limit int, sortDir int) ([]host.Host, error) {
	hostRes, err := host.GetHostsByFromIdWithStatus(id, status, limit, sortDir)
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

// FindHosts uses the service layer's host type to query the backing database for
// the hosts.
func (hc *MockHostConnector) FindHostsById(id, status string, limit int, sort int) ([]host.Host, error) {
	// loop until the key is found

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
