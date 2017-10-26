package openstack

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"

	"github.com/gophercloud/gophercloud"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// ProviderName is used to distinguish between different cloud providers.
	ProviderName = "openstack"
)

// Manager implements the CloudManager interface for OpenStack.
type Manager struct {
	authOptions  *gophercloud.AuthOptions
	endpointOpts *gophercloud.EndpointOpts
	client       client
}

// ProviderSettings specifies the settings used to configure a host instance.
type ProviderSettings struct {
	ImageName     string `mapstructure:"image_name"`
	FlavorName    string `mapstructure:"flavor_name"`
	KeyName       string `mapstructure:"key_name"`
	SecurityGroup string `mapstructure:"security_group"`
}

// Validate verifies a set of ProviderSettings.
func (opts *ProviderSettings) Validate() error {
	if opts.ImageName == "" {
		return errors.New("Image name must not be blank")
	}

	if opts.FlavorName == "" {
		return errors.New("Flavor name must not be blank")
	}

	if opts.KeyName == "" {
		return errors.New("Key name must not be blank")
	}

	if opts.SecurityGroup == "" {
		return errors.New("Security group must not be blank")
	}

	return nil
}

// GetSettings returns an empty ProviderSettings struct since settings are configured on
// instance creation.
func (m *Manager) GetSettings() cloud.ProviderSettings {
	return &ProviderSettings{}
}

//GetInstanceName returns a name to be used for an instance
func (*Manager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
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
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	return nil
}

// SpawnHost attempts to create a new host by requesting one from the OpenStack API.
// Information about the intended (and eventually created) host is recorded in a DB document.
//
// ProviderSettings in the distro should have the following settings:
//     - ImageName:     image name
//     - FlavorName:    like an AWS instance type i.e. m1.large
//     - KeyName:       (optional) keypair name associated with the account
//     - SecurityGroup: (optional) security group name
func (m *Manager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != ProviderName {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			ProviderName, h.Distro.Id, h.Distro.Provider)
	}

	settings := &ProviderSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %s", h.Distro.Id)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid settings in distro %s", h.Distro.Id)
	}

	// Start the instance, and remove the intent host document if unsuccessful.
	opts := getSpawnOptions(h, settings)
	server, err := m.client.CreateInstance(opts, settings.KeyName)
	if err != nil {
		grip.Error(err)
		if rmErr := h.Remove(); rmErr != nil {
			grip.Errorf("Could not remove intent host: %s", message.Fields{
				"host":  h.Id,
				"error": rmErr,
			})
		}
		return nil, errors.Wrapf(err, "Could not start new instance for distro '%s'", h.Distro.Id)
	}

	// Update the ID of the host with the real one
	h.Id = server.ID

	grip.Debug(message.Fields{"message": "new openstack host", "instance": h.Id, "object": h})
	event.LogHostStarted(h.Id)

	return h, nil
}

// CanSpawn always returns true for now.
//
// The OpenStack provider is not always able to spawn new instances if, for example, it has
// exceeded its number of instances, VCPUs, or RAM. Unfortunately, there is no way to know.
func (m *Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *Manager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	server, err := m.client.GetInstance(host.Id)
	if err != nil {
		return cloud.StatusUnknown, err
	}

	return osStatusToEvgStatus(server.Status), nil
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *Manager) TerminateInstance(host *host.Host) error {
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", host.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.DeleteInstance(host.Id); err != nil {
		return errors.Wrap(err, "API call to delete instance failed")
	}

	// Set the host status as terminated and update its termination time
	return host.Terminate()
}

// IsUp checks whether the provisioned host is running.
func (m *Manager) IsUp(host *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(host)
	if err != nil {
		return false, err
	}

	return status == cloud.StatusRunning, nil
}

// OnUp does nothing since tags are attached in SpawnInstance.
func (m *Manager) OnUp(host *host.Host) error {
	return nil
}

// IsSSHReachable returns true if the host can successfully accept and run an SSH command.
func (m *Manager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	opts, err := m.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}

	return hostutil.CheckSSHResponse(context.TODO(), host, opts)
}

// GetDNSName returns the private IP address of the host.
func (m *Manager) GetDNSName(host *host.Host) (string, error) {
	server, err := m.client.GetInstance(host.Id)
	if err != nil {
		return "", err
	}

	for _, subnet := range server.Addresses {
		addresses, ok := subnet.([]interface{})
		if !ok {
			return "", errors.Errorf(
				"type conversion of %+v to []interface{} for host %s", subnet, host.Id)
		}

		for _, address := range addresses {
			keyvalues, ok := address.(map[string]interface{})
			if !ok {
				return "", errors.Errorf(
					"type conversion of %+v to map[string]interface{} for host %s", address, host.Id)
			}

			if ip := keyvalues["addr"]; ip != nil {
				ip, ok = ip.(string)
				if !ok {
					return "", errors.Errorf(
						"type conversion of %+v to string for host %s", ip, host.Id)
				}
				return ip.(string), nil
			}
		}
	}

	return "", errors.Errorf("could not find IP for host %s", host.Id)
}

// GetSSHOptions generates the command line args to be passed to SSH to allow connection
// to the machine.
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

// TimeTilNextPayment always returns 0. The OpenStack dashboard requires third-party
// plugins for billing, monitoring, and other management tools.
func (m *Manager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
