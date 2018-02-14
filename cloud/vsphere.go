// +build go1.7

package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// vsphereManager implements the CloudManager interface for vSphere.
type vsphereManager struct {
	client vsphereClient
}

// vsphereSettings specifies the settings used to configure a host instance.
type vsphereSettings struct {
	Template string `mapstructure:"template"`

	Datastore    string `mapstructure:"datastore"`
	ResourcePool string `mapstructure:"resource_pool"`

	NumCPUs  int32 `mapstructure:"num_cpus"`
	MemoryMB int64 `mapstructure:"memory_mb"`
}

// Validate verifies a set of ProviderSettings.
func (opts *vsphereSettings) Validate() error {
	if opts.Template == "" {
		return errors.New("template must not be blank")
	}

	if opts.ResourcePool == "" {
		opts.ResourcePool = "*" // default path
	}

	if opts.NumCPUs < 0 {
		return errors.New("number of CPUs must be non-negative")
	}

	if opts.MemoryMB < 0 {
		return errors.New("memory in Mb must be non-negative")
	}

	return nil
}

//GetInstanceName returns a name to be used for an instance
func (*vsphereManager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

// GetSettings returns an empty vsphereSettings struct
// since settings are configured on instance creation.
func (m *vsphereManager) GetSettings() ProviderSettings {
	return &vsphereSettings{}
}

// Configure loads the necessary credentials from the global config object.
func (m *vsphereManager) Configure(ctx context.Context, s *evergreen.Settings) error {
	ao := authOptions(s.Providers.VSphere)

	if m.client == nil {
		m.client = &vsphereClientImpl{}
	}

	if err := m.client.Init(ctx, &ao); err != nil {
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	return nil
}

// SpawnHost attempts to create a new host by requesting one from the vSphere API.
// Information about the intended (and eventually created) host is recorded in a DB document.
//
// vsphereSettings in the distro should have the following settings:
//     - Template     (string): name of the template VM
//     - Datastore    (string): (optional) name/path of the datastore to attach to e.g. 1TB_SSD
//     - ResourcePool (string): (optional) name/path of a resource pool e.g. Resources
//     - NumCPUs      (int32):  (optional) number of CPUs e.g. 2
//     - MemoryMB     (int64):  (optional) memory in MB e.g. 2048
//
// Optional fields use the default values of the template vm if not specified.
//     -
func (m *vsphereManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameVsphere {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameVsphere, h.Distro.Id, h.Distro.Provider)
	}

	s := &vsphereSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, s); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %s", h.Distro.Id)
	}

	if err := s.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid settings in distro %s", h.Distro.Id)
	}

	// Start the instance, and remove the intent host document if unsuccessful.
	if _, err := m.client.CreateInstance(ctx, h, s); err != nil {
		if rmErr := h.Remove(); rmErr != nil {
			grip.Errorf("Could not remove intent host '%s': %+v", h.Id, rmErr)
		}
		grip.Error(err)
		return nil, errors.Wrapf(err, "Could not start new instance for distro '%s'", h.Distro.Id)
	}

	grip.Debug(message.Fields{
		"message":  "spawned new instance",
		"instance": h.Id,
		"distro":   h.Distro.Id,
		"provider": h.Provider,
		"object":   h,
	})
	event.LogHostStarted(h.Id)

	return h, nil
}

// CanSpawn always returns true.
func (m *vsphereManager) CanSpawn() (bool, error) {
	return true, nil
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *vsphereManager) GetInstanceStatus(ctx context.Context, host *host.Host) (CloudStatus, error) {
	state, err := m.client.GetPowerState(ctx, host)
	if err != nil {
		return StatusUnknown, errors.Wrapf(err,
			"client failed to get power state for host %s", host.Id)
	}

	return vsphereToEvgStatus(state), nil
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *vsphereManager) TerminateInstance(ctx context.Context, host *host.Host, user string) error {
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", host.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.DeleteInstance(ctx, host); err != nil {
		return errors.Wrapf(err, "API call to delete instance %s failed", host.Id)
	}

	// Set the host status as terminated and update its termination time
	if err := host.Terminate(user); err != nil {
		return errors.Wrapf(err, "could not terminate host %s in db", host.Id)
	}

	return nil
}

// IsUp checks whether the provisioned host is running.
func (m *vsphereManager) IsUp(ctx context.Context, host *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(ctx, host)
	if err != nil {
		return false, errors.Wrapf(err,
			"manager failed to get instance status for host %s", host.Id)
	}

	return status == StatusRunning, nil
}

// OnUp does nothing since tags are attached in SpawnInstance.
func (m *vsphereManager) OnUp(ctx context.Context, host *host.Host) error {
	return nil //TODO
}

// GetDNSName returns the IPv4 address of the host.
func (m *vsphereManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	ip, err := m.client.GetIP(ctx, h)
	if err != nil {
		return "", errors.Wrapf(err, "client failed to get IP for host %s", h.Id)
	}

	return ip, nil
}

// GetSSHOptions generates the command line args to be
// passed to SSH to allow connection to the machine.
func (m *vsphereManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
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
func (m *vsphereManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
