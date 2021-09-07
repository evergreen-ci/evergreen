package cocoa

import (
	"context"

	"github.com/mongodb/grip"
)

// Vault allows you to interact with a secrets storage service.
type Vault interface {
	// CreateSecret creates a new secret and returns the unique identifier for
	// the stored secret. If the secret already exists, it just returns the
	// unique identifier for the existing secret without modifying its value. To
	// update the secret's value, see UpdateValue.
	CreateSecret(ctx context.Context, s NamedSecret) (id string, err error)
	// GetValue returns the value of the secret identified by ID.
	GetValue(ctx context.Context, id string) (val string, err error)
	// UpdateValue updates an existing secret's value by ID.
	UpdateValue(ctx context.Context, s NamedSecret) error
	// DeleteSecret deletes a secret by ID.
	DeleteSecret(ctx context.Context, id string) error
}

// NamedSecret represents a secret with a name.
type NamedSecret struct {
	// Name is either the friendly human-readable name to assign to the secret
	// or the resource name.
	Name *string
	// Value is the stored value of the secret.
	Value *string
}

// NewNamedSecret returns a new uninitialized named secret.
func NewNamedSecret() *NamedSecret {
	return &NamedSecret{}
}

// SetName sets the friendly name for the secret.
func (s *NamedSecret) SetName(name string) *NamedSecret {
	s.Name = &name
	return s
}

// SetValue sets the secret value.
func (s *NamedSecret) SetValue(value string) *NamedSecret {
	s.Value = &value
	return s
}

// Validate checks that both the name and value for the secret are set.
func (s *NamedSecret) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(s.Name == nil, "must specify a name")
	catcher.NewWhen(s.Name != nil && *s.Name == "", "cannot specify an empty name")
	catcher.NewWhen(s.Value == nil, "must specify a value")
	return catcher.Resolve()
}
