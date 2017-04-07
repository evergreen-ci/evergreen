package servicecontext

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/model/host"
)

// DBHostConnector is a struct that implements the Host related methods
// from the ServiceContext through interactions with he backing database.
type DBHostConnector struct{}

// FindHosts uses the service layer's host type to query the backing database for
// the hosts.
func (hc *DBHostConnector) FindHostsById(id string, limit int, sort int) ([]host.Host, error) {
	t, err := host.Find(host.ByAfterId(id, sort).Limit(limit))
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "no hosts found",
		}
	}
	return t, nil
}

// MockHostConnector is a struct that implements the Host related methods
// from the ServiceContext through interactions with he backing database.
type MockHostConnector struct {
	CachedHosts []host.Host
}

// FindHosts uses the service layer's host type to query the backing database for
// the hosts.
func (hc *MockHostConnector) FindHostsById(id string, limit int, sort int) ([]host.Host, error) {
	// loop until the key is found

	for ix, h := range hc.CachedHosts {
		if h.Id == id {
			// We've found the host
			hostsToReturn := []host.Host{}
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
