package data

import (
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/pkg/errors"
)

// DBPodConnector implements the pod-related methods from the connector via
// interactions with the database.
type DBPodConnector struct{}

// CheckPodSecret checks for a pod with a matching ID and secret in the
// database.
func (c *DBPodConnector) CheckPodSecret(id, secret string) error {
	p, err := pod.FindOneByID(id)
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("pod does not exist")
	}
	if secret != p.Secret {
		return errors.New("pod secrets do not match")
	}
	return nil
}

type MockPodConnector struct {
	CachedPods []pod.Pod
}

func (c *MockPodConnector) CheckPodSecret(id, secret string) error {
	for _, p := range c.CachedPods {
		if id != p.ID {
			continue
		}
		if secret != p.Secret {
			return errors.New("pod secrets do not match")
		}
	}
	return errors.New("pod does not exist")
}
