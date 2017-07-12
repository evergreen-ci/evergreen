// +build go1.7

package vsphere

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"

	"github.com/pkg/errors"
)

const (
	// ProviderName is used to distinguish between different cloud providers.
	ProviderName = "vsphere"
)

// Manager implements the CloudManager interface for vSphere.
type Manager struct {
	client client
}

// ProviderSettings specifies the settings used to configure a host instance.
type ProviderSettings struct {}

// Validate verifies a set of ProviderSettings.
func (opts *ProviderSettings) Validate() error {
	return nil //TODO
}

// GetSettings returns an empty ProviderSettings struct
// since settings are configured on instance creation.
func (m *Manager) GetSettings() cloud.ProviderSettings {
	return &ProviderSettings{}
}

// Configure loads the necessary credentials from the global config object.
func (m *Manager) Configure(s *evergreen.Settings) error {
	ao := authOptions(s.Providers.VSphere)

	if m.client == nil {
		m.client = &clientImpl{}
	}

	if err := m.client.Init(&ao); err != nil {
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	return nil
}

// SpawnInstance attempts to create a new host by requesting one from the vSphere API.
// Information about the intended (and eventually created) host is recorded in a DB document.
//
// ProviderSettings in the distro should have the following settings:
//     - 
func (m *Manager) SpawnInstance(d *distro.Distro, hostOpts cloud.HostOptions) (*host.Host, error) {
	return nil, nil //TODO
}

// CanSpawn always returns true.
func (m *Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *Manager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	state, err := m.client.GetPowerState(host)
	if err != nil {
		return cloud.StatusUnknown, errors.Wrap(err, "client failed to get power state")
	}

	return toEvgStatus(state), nil
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *Manager) TerminateInstance(host *host.Host) error {
	return nil //TODO
}

// IsUp checks whether the provisioned host is running.
func (m *Manager) IsUp(host *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(host)
	if err != nil {
		return false, errors.Wrap(err, "manager failed to get instance status")
	}

	return status == cloud.StatusRunning, nil
}

// OnUp does nothing since tags are attached in SpawnInstance.
func (m *Manager) OnUp(host *host.Host) error {
	return nil //TODO
}

// IsSSHReachable returns true if the host can successfully accept and run an SSH command.
func (m *Manager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	opts, err := m.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to get SSH options")
	}

	return hostutil.CheckSSHResponse(host, opts)
}

// GetDNSName returns the IPv4 address of the host.
func (m *Manager) GetDNSName(host *host.Host) (string, error) {
	ip, err := m.client.GetIP(host)
	if err != nil {
		return "", errors.Wrap(err, "client failed to get IP")
	}

	return ip, nil
}

// GetSSHOptions generates the command line args to be
// passed to SSH to allow connection to the machine.
func (m *Manager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, errors.New("No key specified for host")
	}

	opts := []string{"-i", keyPath}
	for _, opt := range host.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}

	return opts, nil
}

// TimeTilNextPayment ...
// TODO: implement payment information for vSphere
func (m *Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
