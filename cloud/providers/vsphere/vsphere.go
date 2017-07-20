// +build go1.7

package vsphere

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
type ProviderSettings struct {
	Template string `mapstructure:"template"`

	Datastore    string `mapstructure:"datastore"`
	ResourcePool string `mapstructure:"resource_pool"`

	NumCPUs  int32 `mapstructure:"num_cpus"`
	MemoryMB int64 `mapstructure:"memory_mb"`
}

// Validate verifies a set of ProviderSettings.
func (opts *ProviderSettings) Validate() error {
	if opts.Template == "" {
		return errors.New("template must not be blank")
	}

	if opts.ResourcePool == "" {
		opts.ResourcePool = "*"  // default path
	}

	if opts.NumCPUs < 0 {
		return errors.New("number of CPUs must be non-negative")
	}

	if opts.MemoryMB < 0 {
		return errors.New("memory in Mb must be non-negative")
	}

	return nil
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
//     - Template     (string): name of the template VM
//     - Datastore    (string): (optional) name/path of the datastore to attach to e.g. 1TB_SSD
//     - ResourcePool (string): (optional) name/path of a resource pool e.g. XSERVE_Cluster
//     - NumCPUs      (int32):  (optional) number of CPUs e.g. 2
//     - MemoryMB     (int64):  (optional) memory in MB e.g. 2048
//
// Optional fields use the default values of the template vm if not specified.
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

	// Proactively record all information about the host we want to create. This way, if we are
	// unable to start it or record its instance ID, we have a way of knowing what went wrong.
	name := d.GenerateName()
	intentHost := cloud.NewIntent(*d, name, ProviderName, hostOpts)
	if err := intentHost.Insert(); err != nil {
		err = errors.Wrapf(err, "Could not insert intent host '%s'", intentHost.Id)
		grip.Error(err)
		return nil, err
	}

	grip.Debug(message.Fields{
		"message": "inserted intent host to signal instance spawn intent",
		"intent_host": name,
		"distro": d.Id,
	})

	// Start the instance, and remove the intent host document if unsuccessful.
	if _, err := m.client.CreateInstance(intentHost, s); err != nil {
		if rmErr := intentHost.Remove(); rmErr != nil {
			grip.Errorf("Could not remove intent host '%s': %+v", intentHost.Id, rmErr)
		}
		grip.Error(err)
		return nil, errors.Wrapf(err, "Could not start new instance for distro '%s'", d.Id)
	}

	grip.Debug(message.Fields{
		"message": "spawned new instance",
		"instance": name,
		"object": intentHost,
	})

	return intentHost, nil
}

// CanSpawn always returns true.
func (m *Manager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *Manager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	state, err := m.client.GetPowerState(host)
	if err != nil {
		return cloud.StatusUnknown, errors.Wrapf(err,
			"client failed to get power state for host %s", host.Id)
	}

	return toEvgStatus(state), nil
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *Manager) TerminateInstance(host *host.Host) error {
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", host.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.DeleteInstance(host); err != nil {
		return errors.Wrapf(err, "API call to delete instance %s failed", host.Id)
	}

	// Set the host status as terminated and update its termination time
	if err := host.Terminate(); err != nil {
		return errors.Wrapf(err, "could not terminate host %s in db", host.Id)
	}

	return nil
}

// IsUp checks whether the provisioned host is running.
func (m *Manager) IsUp(host *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(host)
	if err != nil {
		return false, errors.Wrapf(err,
			"manager failed to get instance status for host %s", host.Id)
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
		return false, errors.Wrapf(err, "failed to get SSH options for host %s", host.Id)
	}

	return hostutil.CheckSSHResponse(host, opts)
}

// GetDNSName returns the IPv4 address of the host.
func (m *Manager) GetDNSName(host *host.Host) (string, error) {
	ip, err := m.client.GetIP(host)
	if err != nil {
		return "", errors.Wrapf(err, "client failed to get IP for host %s", host.Id)
	}

	return ip, nil
}

// GetSSHOptions generates the command line args to be
// passed to SSH to allow connection to the machine.
func (m *Manager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, errors.Errorf("No key specified for host %s", host.Id)
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
