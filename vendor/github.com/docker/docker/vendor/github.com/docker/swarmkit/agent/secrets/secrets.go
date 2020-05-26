package secrets

import (
	"fmt"
	"sync"

	"github.com/docker/swarmkit/agent/exec"
	"github.com/docker/swarmkit/api"
	"github.com/docker/swarmkit/identity"
)

// secrets is a map that keeps all the currently available secrets to the agent
// mapped by secret ID.
type secrets struct {
	mu sync.RWMutex
	m  map[string]*api.Secret
}

// NewManager returns a place to store secrets.
func NewManager() exec.SecretsManager {
	return &secrets{
		m: make(map[string]*api.Secret),
	}
}

// Get returns a secret by ID.  If the secret doesn't exist, returns nil.
func (s *secrets) Get(secretID string) (*api.Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s, ok := s.m[secretID]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("secret %s not found", secretID)
}

// Add adds one or more secrets to the secret map.
func (s *secrets) Add(secrets ...api.Secret) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, secret := range secrets {
		s.m[secret.ID] = secret.Copy()
	}
}

// Remove removes one or more secrets by ID from the secret map.  Succeeds
// whether or not the given IDs are in the map.
func (s *secrets) Remove(secrets []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, secret := range secrets {
		delete(s.m, secret)
	}
}

// Reset removes all the secrets.
func (s *secrets) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = make(map[string]*api.Secret)
}

// taskRestrictedSecretsProvider restricts the ids to the task.
type taskRestrictedSecretsProvider struct {
	secrets   exec.SecretGetter
	secretIDs map[string]struct{} // allow list of secret ids
	taskID    string              // ID of the task the provider restricts for
}

func (sp *taskRestrictedSecretsProvider) Get(secretID string) (*api.Secret, error) {
	if _, ok := sp.secretIDs[secretID]; !ok {
		return nil, fmt.Errorf("task not authorized to access secret %s", secretID)
	}

	// First check if the secret is available with the task specific ID, which is the concatenation
	// of the original secret ID and the task ID with a dot in between.
	// That is the case when a secret driver has returned DoNotReuse == true for a secret value.
	taskSpecificID := identity.CombineTwoIDs(secretID, sp.taskID)
	secret, err := sp.secrets.Get(taskSpecificID)
	if err != nil {
		// Otherwise, which is the default case, the secret is retrieved by its original ID.
		return sp.secrets.Get(secretID)
	}
	// For all intents and purposes, the rest of the flow should deal with the original secret ID.
	secret.ID = secretID
	return secret, err
}

// Restrict provides a getter that only allows access to the secrets
// referenced by the task.
func Restrict(secrets exec.SecretGetter, t *api.Task) exec.SecretGetter {
	sids := map[string]struct{}{}

	container := t.Spec.GetContainer()
	if container != nil {
		for _, ref := range container.Secrets {
			sids[ref.SecretID] = struct{}{}
		}
	}

	return &taskRestrictedSecretsProvider{secrets: secrets, secretIDs: sids, taskID: t.ID}
}
