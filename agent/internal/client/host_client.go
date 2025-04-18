package client

import (
	"time"

	"github.com/evergreen-ci/evergreen"
)

const (
	defaultMaxAttempts  = 10
	defaultTimeoutStart = time.Second * 2
	defaultTimeoutMax   = time.Minute * 10
	heartbeatTimeout    = time.Minute * 1
	restartFailedTimout = time.Minute * 1
)

// hostCommunicator implements Communicator and makes requests to API endpoints
// for an agent running on a host.
type hostCommunicator struct {
	baseCommunicator

	hostID     string
	hostSecret string
}

// NewHostCommunicator returns a Communicator capable of making HTTP REST
// requests against the API server for an agent running on a host. To change
// the default retry behavior, use the SetTimeoutStart, SetTimeoutMax, and
// SetMaxAttempts methods.
func NewHostCommunicator(serverURL, hostID, hostSecret string) Communicator {
	c := &hostCommunicator{
		baseCommunicator: newBaseCommunicator(serverURL, map[string]string{
			evergreen.HostHeader:       hostID,
			evergreen.HostSecretHeader: hostSecret,
		}),
		hostID:     hostID,
		hostSecret: hostSecret,
	}

	c.resetClient()

	return c
}
