// +build go1.7

package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// vsphereManager implements the Manager interface for vSphere.
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
	if h.Distro.ProviderSettings != nil {
		if err := mapstructure.Decode(h.Distro.ProviderSettings, s); err != nil {
			return nil, errors.Wrapf(err, "Error decoding params for distro %s", h.Distro.Id)
		}
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

func (m *vsphereManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances for vsphere provider")
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
func (m *vsphereManager) TerminateInstance(ctx context.Context, host *host.Host, user, reason string) error {
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", host.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.DeleteInstance(ctx, host); err != nil {
		return errors.Wrapf(err, "API call to delete instance %s failed", host.Id)
	}

	// Set the host status as terminated and update its termination time
	if err := host.Terminate(user, reason); err != nil {
		return errors.Wrapf(err, "could not terminate host %s in db", host.Id)
	}

	return nil
}

func (m *vsphereManager) StopInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StopInstance is not supported for vsphere provider")
}

func (m *vsphereManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for vsphere provider")
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

func (m *vsphereManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with vsphere provider")
}

func (m *vsphereManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with vsphere provider")
}

func (m *vsphereManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volumes with vsphere provider")
}

func (m *vsphereManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volumes with vsphere provider")
}

// GetDNSName returns the IPv4 address of the host.
func (m *vsphereManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	ip, err := m.client.GetIP(ctx, h)
	if err != nil {
		return "", errors.Wrapf(err, "client failed to get IP for host %s", h.Id)
	}

	return ip, nil
}

// TimeTilNextPayment ...
// TODO: implement payment information for vSphere
func (m *vsphereManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
