package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/oauth2/jwt"
)

const (
	// ProviderName is used to distinguish between different cloud providers.
	ProviderName = "gce"
)

// gceManager implements the Manager interface for Google Compute Engine.
type gceManager struct {
	client gceClient
}

// GCESettings specifies the settings used to configure a host instance.
type GCESettings struct {
	Project string `mapstructure:"project_id" json:"project_id" bson:"project_id"`
	Zone    string `mapstructure:"zone" json:"zone" bson:"zone"`

	ImageName   string `mapstructure:"image_name" json:"image_name" bson:"image_name"`
	ImageFamily string `mapstructure:"image_family" json:"image_family" bson:"image_family"`

	MachineName string `mapstructure:"instance_type" json:"instance_type" bson:"instance_type"`
	NumCPUs     int64  `mapstructure:"num_cpus" json:"num_cpus" bson:"num_cpus"`
	MemoryMB    int64  `mapstructure:"memory_mb" json:"memory_mb" bson:"memory_mb"`

	DiskType   string `mapstructure:"disk_type" json:"disk_type" bson:"disk_type"`
	DiskSizeGB int64  `mapstructure:"disk_size_gb" json:"disk_size_gb" bson:"disk_size_gb"`

	// Network tags are used to configure network firewalls.
	NetworkTags []string `mapstructure:"network_tags" json:"network_tags" bson:"network_tags"`

	// By default, GCE uses project-wide SSH keys. Project-wide keys should be manually
	// added to the project metadata. These SSH keys are optional instance-wide keys.
	SSHKeys sshKeyGroup `mapstructure:"ssh_keys" json:"ssh_keys" bson:"ssh_keys"`
}

// Validate verifies a set of GCESettings.
func (opts *GCESettings) Validate() error {
	standardMachine := opts.MachineName != ""
	customMachine := opts.NumCPUs > 0 && opts.MemoryMB > 0

	if standardMachine == customMachine {
		return errors.New("must specify either machine type or num CPUs and memory")
	}

	if (opts.ImageFamily == "") == (opts.ImageName == "") {
		return errors.New("exactly one of image family or image name must be blank")
	}

	if opts.DiskType == "" {
		return errors.New("disk type must not be blank")
	}

	return nil
}

func (opts *GCESettings) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "marshalling provider setting into BSON")
		}
		if err := bson.Unmarshal(bytes, opts); err != nil {
			return errors.Wrap(err, "unmarshalling BSON into provider settings")
		}
	}
	return nil
}

// Configure loads the necessary credentials from the global config object.
func (m *gceManager) Configure(ctx context.Context, s *evergreen.Settings) error {
	config := s.Providers.GCE

	jwtConfig := &jwt.Config{
		Email:        config.ClientEmail,
		PrivateKey:   []byte(config.PrivateKey),
		PrivateKeyID: config.PrivateKeyID,
		Scopes:       []string{computeScope},
		TokenURL:     config.TokenURI,
	}

	if m.client == nil {
		m.client = &gceClientImpl{}
	}

	if err := m.client.Init(ctx, jwtConfig); err != nil {
		return errors.Wrap(err, "initializing client connection")
	}

	return nil
}

// SpawnHost attempts to create a new host by requesting one from the Google Compute API.
// Information about the intended (and eventually created) host is recorded in a DB document.
//
// ProviderSettings in the distro should have the following settings:
//
//   - Project: Google Compute project ID
//
//   - Zone:    project zone i.e. us-east1-c
//
//     (Exactly one of ImageName or ImageFamily must be specified)
//
//   - ImageName:   the disk will use the private image of the specified name
//
//   - ImageFamily: the disk will use the newest image from a private image family
//
//     (Either MachineName OR both NumCPUs and MemoryMB must be specified)
//
//   - MachineName: instance type i.e. n1-standard-8
//
//   - NumCPUs:     number of cores i.e. 2
//
//   - MemoryMB:    memory, in MB i.e. 1024
//
//   - DiskSizeGB:  boot disk size, in base-2 GB
//
//   - DiskType:    boot disk type i.e. pd-standard
//
//   - NetworkTags: (optional) security groups
//
//   - SSHKeys:     username-key pairs
func (m *gceManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != ProviderName {
		return nil, errors.Errorf("spawning instance for distro '%s': distro provider is '%s'", h.Distro.Id, h.Distro.Provider)
	}

	s := &GCESettings{}
	if err := s.FromDistroSettings(h.Distro, ""); err != nil {
		return nil, errors.Wrapf(err, "decoding params for distro '%s'", h.Distro.Id)
	}
	if err := s.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid provider settings in distro '%s'", h.Distro.Id)
	}

	grip.Debugf("Settings validated for distro %s", h.Distro.Id)

	// Proactively record all information about the host we want to create. This way, if we are
	// unable to start it or record its instance ID, we have a way of knowing what went wrong.
	// the document is updated later in hostinit, rather than here
	h.Zone = s.Zone
	h.Project = s.Project

	// Start the instance, and remove the intent host document if unsuccessful.
	if _, err := m.client.CreateInstance(h, s); err != nil {
		if rmErr := h.Remove(); rmErr != nil {
			grip.Error(message.WrapError(rmErr, message.Fields{
				"message": "could not remove intent host",
				"host_id": h.Id,
			}))
		}
		grip.Error(err)
		return nil, errors.Wrapf(err, "starting new instance for distro '%s'", h.Distro.Id)
	}

	grip.Debug(message.Fields{"message": "new gce host", "instance": h.Id, "object": h})
	return h, nil
}

func (m *gceManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances with GCE provider")
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *gceManager) GetInstanceStatus(ctx context.Context, host *host.Host) (CloudStatus, error) {
	instance, err := m.client.GetInstance(host)
	if err != nil {
		return StatusUnknown, err
	}

	return gceToEvgStatus(instance.Status), nil
}

func (m *gceManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with GCE provider")
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *gceManager) TerminateInstance(ctx context.Context, host *host.Host, user, reason string) error {
	if host.Status == evergreen.HostTerminated {
		return errors.Errorf("cannot terminate host '%s' because it's already marked as terminated", host.Id)
	}

	if err := m.client.DeleteInstance(host); err != nil {
		return errors.Wrap(err, "deleting instance")
	}

	// Set the host status as terminated and update its termination time
	return host.Terminate(user, reason)
}

func (m *gceManager) StopInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StopInstance is not supported for GCE provider")
}

func (m *gceManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for GCE provider")
}

// IsUp checks whether the provisioned host is running.
func (m *gceManager) IsUp(ctx context.Context, host *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(ctx, host)
	if err != nil {
		return false, err
	}

	return status == StatusRunning, nil
}

// OnUp does nothing since tags are attached in SpawnInstance.
func (m *gceManager) OnUp(context.Context, *host.Host) error {
	return nil
}

func (m *gceManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with GCE provider")
}

func (m *gceManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with GCE provider")
}

func (m *gceManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with GCE provider")
}

func (m *gceManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with GCE provider")
}

func (m *gceManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with GCE provider")
}

func (m *gceManager) GetVolumeAttachment(context.Context, string) (*host.VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with GCE provider")
}

func (m *gceManager) CheckInstanceType(context.Context, string) error {
	return errors.New("can't specify instance type with GCE provider")
}

// Cleanup is a noop for the gce provider.
func (m *gceManager) Cleanup(context.Context) error {
	return nil
}

// GetDNSName returns the external IPv4 address of the host.
func (m *gceManager) GetDNSName(ctx context.Context, host *host.Host) (string, error) {
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

const (
	// gceMinUptime is the minimum time to run a host before shutting it down.
	gceMinUptime = 30 * time.Minute
	// gceBufferTime is the time to leave an idle host running after it has completed
	// a task before shutting it down, given that it has already run for MinUptime.
	gceBufferTime = 10 * time.Minute
)

// TimeTilNextPayment returns the time until when the host should be destroyed.
//
// In general, TimeTilNextPayment aims to run hosts for a minimum of MinUptime, but
// leaves a buffer time of at least BufferTime after a task has completed before
// shutting a host down. We assume the host is currently free (not running a task).
func (m *gceManager) TimeTilNextPayment(h *host.Host) time.Duration {
	now := time.Now()

	// potential end times given certain milestones in the host lifespan
	endMinUptime := h.CreationTime.Add(gceMinUptime)
	endBufferTime := h.LastTaskCompletedTime.Add(gceBufferTime)

	// durations until the potential end times
	tilEndMinUptime := endMinUptime.Sub(now)
	tilEndBufferTime := endBufferTime.Sub(now)

	// check that last task completed time is not zero
	if utility.IsZeroTime(h.LastTaskCompletedTime) {
		return tilEndMinUptime
	}

	// return the greater of the two durations
	if tilEndBufferTime.Minutes() < tilEndMinUptime.Minutes() {
		return tilEndMinUptime
	}
	return tilEndBufferTime
}

func (m *gceManager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	return nil
}
