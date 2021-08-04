package client

import "github.com/evergreen-ci/evergreen"

// podCommunicator implements Communicator and makes requests to API endpoints
// for an agent running in a pod.
type podCommunicator struct {
	baseCommunicator

	podID     string
	podSecret string
}

// NewPodCommunicator returns a Communicator capable of making HTTP requests
// against the API server for an agent running in a pod.
func NewPodCommunicator(serverURL, podID, podSecret string) Communicator {
	c := &podCommunicator{
		baseCommunicator: newBaseCommunicator(serverURL, map[string]string{
			evergreen.PodHeader:       podID,
			evergreen.PodSecretHeader: podSecret,
		}),
		podID:     podID,
		podSecret: podSecret,
	}

	c.resetClient()

	return c
}
