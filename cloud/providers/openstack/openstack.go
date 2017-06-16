package openstack

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"

	"github.com/gophercloud/gophercloud"
	"github.com/pkg/errors"
)

const (
	// ProviderName is used to distinguish between different cloud providers.
	ProviderName = "OpenStack"
)

// Manager implements the CloudManager interface for OpenStack.
type Manager struct {
	authOptions  *gophercloud.AuthOptions
	endpointOpts *gophercloud.EndpointOpts
	client       client
}

// ProviderSettings specifies the settings used to configure a host instance.
type ProviderSettings struct {
	ImageName      string
	FlavorName     string
	SecurityGroups []string
}

// Validate verifies a set of ProviderSettings.
func (opts *ProviderSettings) Validate() error {
	if opts.ImageName == "" {
		return errors.New("Image name must not be blank")
	}

	if opts.FlavorName == "" {
		return errors.New("Flavor name must not be blank")
	}

	return nil
}

// GetSettings does nothing.
func (m *Manager) GetSettings() cloud.ProviderSettings {
	return &ProviderSettings{}
}

// Configure loads the necessary credentials from the global config object.
func (m *Manager) Configure(s *evergreen.Settings) error {
	config := s.Providers.OpenStack

	m.authOptions = &gophercloud.AuthOptions{
		IdentityEndpoint: config.IdentityEndpoint,
		Username:         config.Username,
		Password:         config.Password,
		DomainName:       config.DomainName,
		TenantName:       config.ProjectName,
		TenantID:         config.ProjectID,
	}

	m.endpointOpts = &gophercloud.EndpointOpts{
		Region: config.Region,
	}

	if m.client == nil {
		m.client = &clientImpl{}
	}

	if err := m.client.Init(*m.authOptions, *m.endpointOpts); err != nil {
		return err
	}

	return nil
}

func (m *Manager) SpawnInstance(d *distro.Distro, opts cloud.HostOptions) (*host.Host, error) {
	return nil, nil
}

func (m *Manager) CanSpawn() (bool, error) {
	return true, nil
}

func (m *Manager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	return cloud.StatusUnknown, nil
}

func (m *Manager) TerminateInstance(host *host.Host) error {
	return nil
}

func (m *Manager) IsUp(host *host.Host) (bool, error) {
	return false, nil
}

func (m *Manager) OnUp(host *host.Host) error {
	return nil
}

func (m *Manager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	return false, nil
}

func (m *Manager) GetDNSName(host *host.Host) (string, error) {
	return "", nil
}

func (m *Manager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	return []string{}, nil
}

func (m *Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}