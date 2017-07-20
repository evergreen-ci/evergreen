// +build go1.7

package gce

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/oauth2/jwt"
)

const (
	// ProviderName is used to distinguish between different cloud providers.
	ProviderName = "gce"
)

// Manager implements the CloudManager interface for Google Compute Engine.
type Manager struct {
	client client
}

// ProviderSettings specifies the settings used to configure a host instance.
type ProviderSettings struct {
	Project string `mapstructure:"project_id"`
	Zone    string `mapstructure:"zone"`

	ImageName   string `mapstructure:"image_name"`
	ImageFamily string `mapstructure:"image_family"`

	MachineName string `mapstructure:"instance_type"`
	NumCPUs     int64  `mapstructure:"num_cpus"`
	MemoryMB    int64  `mapstructure:"memory_mb"`

	DiskType    string `mapstructure:"disk_type"`
	DiskSizeGB  int64  `mapstructure:"disk_size_gb"`

	// By default, GCE uses project-wide SSH keys. Project-wide keys should be manually
	// added to the project metadata. These SSH keys are optional instance-wide keys.
	SSHKeys sshKeyGroup `mapstructure:"ssh_keys"`
}

// Validate verifies a set of ProviderSettings.
func (opts *ProviderSettings) Validate() error {

	standardMachine := opts.MachineName != ""
	customMachine := opts.NumCPUs > 0 && opts.MemoryMB > 0

	if standardMachine == customMachine {
		return errors.New("Must specify either machine type OR num CPUs and memory")
	}

	if (opts.ImageFamily == "") == (opts.ImageName == "") {
		return errors.New("Exactly one of image family or image name must be blank")
	}

	if opts.DiskType == "" {
		return errors.New("Disk type must not be blank")
	}

	return nil
}

// GetSettings returns an empty ProviderSettings struct since settings are configured on
// instance creation.
func (m *Manager) GetSettings() cloud.ProviderSettings {
	return &ProviderSettings{}
}

// Configure loads the necessary credentials from the global config object.
func (m *Manager) Configure(s *evergreen.Settings) error {
	config := s.Providers.GCE

	jwtConfig := &jwt.Config{
		Email:        config.ClientEmail,
		PrivateKey:   []byte(config.PrivateKey),
		PrivateKeyID: config.PrivateKeyID,
		Scopes:       []string{computeScope},
		TokenURL:     config.TokenURI,
	}

	if m.client == nil {
		m.client = &clientImpl{}
	}

	if err := m.client.Init(jwtConfig); err != nil {
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	return nil
}

// SpawnInstance attempts to create a new host by requesting one from the Google Compute API.
// Information about the intended (and eventually created) host is recorded in a DB document.
//
// ProviderSettings in the distro should have the following settings:
//     - Project: Google Compute project ID
//     - Zone:    project zone i.e. us-east1-c
//
//     (Exactly one of ImageName or ImageFamily must be specified)
//     - ImageName:   the disk will use the private image of the specified name
//     - ImageFamily: the disk will use the newest image from a private image family
//
//     (Either MachineName OR both NumCPUs and MemoryMB must be specified)
//     - MachineName: instance type i.e. n1-standard-8
//     - NumCPUs:     number of cores i.e. 2
//     - MemoryMB:    memory, in MB i.e. 1024
//
//     - DiskSizeGB:  boot disk size, in base-2 GB
//     - DiskType:    boot disk type i.e. pd-standard
//
//     - SSHKeys:     username-key pairs
func (m *Manager) SpawnInstance(d *distro.Distro, hostOpts cloud.HostOptions) (*host.Host, error) {
	if d.Provider != ProviderName {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			ProviderName, d.Id, d.Provider)
	}

	s := &ProviderSettings{}
	if err := mapstructure.Decode(d.ProviderSettings, s); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %s", d.Id)
	}

	if err := s.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid settings in distro %s", d.Id)
	}

	grip.Debugf("Settings validated for distro %s", d.Id)

	// Proactively record all information about the host we want to create. This way, if we are
	// unable to start it or record its instance ID, we have a way of knowing what went wrong.
	name := generateName(d)
	intentHost := cloud.NewIntent(*d, name, ProviderName, hostOpts)
	intentHost.Zone = s.Zone
	intentHost.Project = s.Project
	if err := intentHost.Insert(); err != nil {
		err = errors.Wrapf(err, "Could not insert intent host '%s'", intentHost.Id)
		grip.Error(err)
		return nil, err
	}
	grip.Debugf("Inserted intent host '%s' for distro '%s' to signal instance spawn intent", name, d.Id)

	// Start the instance, and remove the intent host document if unsuccessful.
	if _, err := m.client.CreateInstance(intentHost, s); err != nil {
		if rmErr := intentHost.Remove(); rmErr != nil {
			grip.Errorf("Could not remove intent host '%s': %+v", intentHost.Id, rmErr)
		}
		grip.Error(err)
		return nil, errors.Wrapf(err, "Could not start new instance for distro '%s'", d.Id)
	}

	grip.Debugf("New instance: %v", message.Fields{"instance": name, "object": intentHost})
	return intentHost, nil
}

// CanSpawn always returns true.
func (m *Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *Manager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	instance, err := m.client.GetInstance(host)
	if err != nil {
		return cloud.StatusUnknown, err
	}

	return toEvgStatus(instance.Status), nil
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *Manager) TerminateInstance(host *host.Host) error {
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", host.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.DeleteInstance(host); err != nil {
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

	return hostutil.CheckSSHResponse(host, opts)
}

// GetDNSName returns the external IPv4 address of the host.
func (m *Manager) GetDNSName(host *host.Host) (string, error) {
	instance, err := m.client.GetInstance(host)
	if err != nil {
		return "", err
	}

	if len(instance.NetworkInterfaces) == 0 {
		return "", errors.New("instance is not attached to any network: should be impossible")
	}

	configs := instance.NetworkInterfaces[0].AccessConfigs
	if len(configs) == 0 {
		return "", errors.New("instance does not have any access configs: should be impossible")
	}

	return configs[0].NatIP, nil
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
