// +build go1.7

package cloud

import (
	"context"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
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
	Project string `mapstructure:"project_id"`
	Zone    string `mapstructure:"zone"`

	ImageName   string `mapstructure:"image_name"`
	ImageFamily string `mapstructure:"image_family"`

	MachineName string `mapstructure:"instance_type"`
	NumCPUs     int64  `mapstructure:"num_cpus"`
	MemoryMB    int64  `mapstructure:"memory_mb"`

	DiskType   string `mapstructure:"disk_type"`
	DiskSizeGB int64  `mapstructure:"disk_size_gb"`

	// Network tags are used to configure network firewalls.
	NetworkTags []string `mapstructure:"network_tags"`

	// By default, GCE uses project-wide SSH keys. Project-wide keys should be manually
	// added to the project metadata. These SSH keys are optional instance-wide keys.
	SSHKeys sshKeyGroup `mapstructure:"ssh_keys"`
}

// Validate verifies a set of GCESettings.
func (opts *GCESettings) Validate() error {
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

func (opts *GCESettings) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "error marshalling provider setting into bson")
		}
		if err := bson.Unmarshal(bytes, opts); err != nil {
			return errors.Wrap(err, "error unmarshalling bson into provider settings")
		}
	} else if d.ProviderSettings != nil {
		if err := mapstructure.Decode(d.ProviderSettings, opts); err != nil {
			return errors.Wrapf(err, "Error decoding params for distro %s: %+v", d.Id, opts)
		}
		bytes, err := bson.Marshal(opts)
		if err != nil {
			return errors.Wrap(err, "error marshalling provider setting into bson")
		}
		doc := &birch.Document{}
		if err := doc.UnmarshalBSON(bytes); err != nil {
			return errors.Wrapf(err, "error unmarshalling settings bytes into document")
		}
		if err := d.UpdateProviderSettings(doc); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"distro":   d.Id,
				"provider": d.Provider,
				"settings": d.ProviderSettings,
			}))
			return errors.Wrapf(err, "error updating provider settings")
		}
	}
	return nil
}

// GetSettings returns an empty GCESettings struct since settings are configured on
// instance creation.
func (m *gceManager) GetSettings() ProviderSettings {
	return &GCESettings{}
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
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	return nil
}

// SpawnHost attempts to create a new host by requesting one from the Google Compute API.
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
//     - NetworkTags: (optional) security groups
//     - SSHKeys:     username-key pairs
func (m *gceManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != ProviderName {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			ProviderName, h.Distro.Id, h.Distro.Provider)
	}

	s := &GCESettings{}
	if h.Distro.ProviderSettings != nil {
		if err := s.FromDistroSettings(h.Distro, ""); err != nil {
			return nil, errors.Wrapf(err, "Error decoding params for distro %s", h.Distro.Id)
		}
	}
	if err := s.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid settings in distro %s", h.Distro.Id)
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
			grip.Errorf("Could not remove intent host '%s': %+v", h.Id, rmErr)
		}
		grip.Error(err)
		return nil, errors.Wrapf(err, "Could not start new instance for distro '%s'", h.Distro.Id)
	}

	grip.Debug(message.Fields{"message": "new gce host", "instance": h.Id, "object": h})
	return h, nil
}

func (m *gceManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances with gce provider")
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *gceManager) GetInstanceStatus(ctx context.Context, host *host.Host) (CloudStatus, error) {
	instance, err := m.client.GetInstance(host)
	if err != nil {
		return StatusUnknown, err
	}

	return gceToEvgStatus(instance.Status), nil
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *gceManager) TerminateInstance(ctx context.Context, host *host.Host, user, reason string) error {
	if host.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", host.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.DeleteInstance(host); err != nil {
		return errors.Wrap(err, "API call to delete instance failed")
	}

	// Set the host status as terminated and update its termination time
	return host.Terminate(user, reason)
}

func (m *gceManager) StopInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StopInstance is not supported for gce provider")
}

func (m *gceManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for gce provider")
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
	return errors.New("can't attach volume with gce provider")
}

func (m *gceManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with gce provider")
}

func (m *gceManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with gce provider")
}

func (m *gceManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with gce provider")
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
