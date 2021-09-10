package mock

import (
	"context"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/utility"
)

// Vault provides a mock implementation of a cocoa.Vault backed by any vault by
// default. This makes it possible to introspect on inputs to the vault and
// control the vault's output. It provides some default implementations where
// possible.
type Vault struct {
	cocoa.Vault

	CreateSecretInput  *cocoa.NamedSecret
	CreateSecretOutput *string
	CreateSecretError  error

	GetValueInput  *string
	GetValueOutput *string
	GetValueError  error

	UpdateValueInput *cocoa.NamedSecret
	UpdateValueError error

	DeleteSecretInput *string
	DeleteSecretError error
}

// NewVault creates a mock Vault backed by the given Vault.
func NewVault(v cocoa.Vault) *Vault {
	return &Vault{
		Vault: v,
	}
}

// CreateSecret saves the input options and returns a mock secret ID. The mock
// output can be customized. By default, it will call the backing Vault
// implementation's CreateSecret.
func (m *Vault) CreateSecret(ctx context.Context, s cocoa.NamedSecret) (id string, err error) {
	m.CreateSecretInput = &s

	if m.CreateSecretOutput != nil || m.CreateSecretError != nil {
		return utility.FromStringPtr(m.CreateSecretOutput), m.CreateSecretError
	}

	return m.Vault.CreateSecret(ctx, s)
}

// GetValue saves the input options and returns an existing mock secret's value.
// The mock output can be customized. By default, it will call the backing Vault
// implementation's GetValue.
func (m *Vault) GetValue(ctx context.Context, id string) (val string, err error) {
	m.GetValueInput = &id

	if m.GetValueOutput != nil || m.GetValueError != nil {
		return utility.FromStringPtr(m.GetValueOutput), m.GetValueError
	}

	return m.Vault.GetValue(ctx, id)
}

// UpdateValue saves the input options and updates an existing mock secret. The
// mock output can be customized. By default, it will call the backing Vault
// implementation's UpdateValue.
func (m *Vault) UpdateValue(ctx context.Context, s cocoa.NamedSecret) error {
	m.UpdateValueInput = &s

	if m.UpdateValueError != nil {
		return m.UpdateValueError
	}

	return m.Vault.UpdateValue(ctx, s)
}

// DeleteSecret saves the input options and deletes an existing mock secret. The
// mock output can be customized. By default, it will call the backing Vault
// implementation's DeleteSecret.
func (m *Vault) DeleteSecret(ctx context.Context, id string) error {
	m.DeleteSecretInput = &id

	if m.DeleteSecretError != nil {
		return m.DeleteSecretError
	}

	return m.Vault.DeleteSecret(ctx, id)
}
