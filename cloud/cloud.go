package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ProviderSettings exposes provider-specific configuration settings for a Manager.
type ProviderSettings interface {
	Validate() error

	// If zone is specified, returns the provider settings for that region.
	// This is currently only being implemented for EC2 hosts.
	FromDistroSettings(distro.Distro, string) error
}

// Manager is an interface which handles creating new hosts or modifying
// them via some third-party API.
type Manager interface {
	// Load credentials or other settings from the config file
	Configure(context.Context, *evergreen.Settings) error

	// SpawnHost attempts to create a new host by requesting one from the
	// provider's API.
	SpawnHost(context.Context, *host.Host) (*host.Host, error)

	// ModifyHost modifies an existing host
	ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error

	// Gets the state of an instance
	GetInstanceState(context.Context, *host.Host) (CloudInstanceState, error)

	// SetPortMappings sets the port mappings for the container
	SetPortMappings(context.Context, *host.Host, *host.Host) error

	// TerminateInstance destroys the host in the underlying provider and cleans
	// up IP resources associated with the host, if any (see CleanupIP).
	TerminateInstance(context.Context, *host.Host, string, string) error

	// StopInstance stops an instance.
	StopInstance(ctx context.Context, h *host.Host, shouldKeepOff bool, user string) error

	// StartInstance starts a stopped instance.
	StartInstance(context.Context, *host.Host, string) error

	// GetDNSName returns the DNS name of a host.
	GetDNSName(context.Context, *host.Host) (string, error)

	// AttachVolume attaches a volume to a host.
	AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error

	// DetachVolume detaches a volume from a host.
	DetachVolume(context.Context, *host.Host, string) error

	// CreateVolume creates a new volume for attaching to a host.
	CreateVolume(context.Context, *host.Volume) (*host.Volume, error)

	// DeleteVolume deletes a volume.
	DeleteVolume(context.Context, *host.Volume) error

	// ModifyVolume modifies an existing volume.
	ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error

	// GetVolumeAttachment gets a volume's attachment
	GetVolumeAttachment(context.Context, string) (*VolumeAttachment, error)

	// CheckInstanceType determines if the given instance type is available in the current region.
	CheckInstanceType(context.Context, string) error

	// TimeTilNextPayment returns how long there is until the next payment
	// is due for a particular host
	TimeTilNextPayment(*host.Host) time.Duration

	// AllocateIP allocates a new IP address.
	AllocateIP(context.Context) (*host.IPAddress, error)

	// AssociateIP associates an IP address allocated to a host with the host's
	// network interface.
	AssociateIP(context.Context, *host.Host) error

	// CleanupIP cleans up the IP address resources for a host, if it has any.
	// If calling TerminateInstance, CleanupIP is not needed.
	CleanupIP(context.Context, *host.Host) error

	// Cleanup triggers the manager to clean up resources left behind by day-to-day operations.
	Cleanup(context.Context) error
}

type ContainerManager interface {
	Manager

	// GetContainers returns the IDs of all running containers on a specified host
	GetContainers(context.Context, *host.Host) ([]string, error)
	// RemoveOldestImage removes the earliest created image on a specified host
	RemoveOldestImage(ctx context.Context, h *host.Host) error
	// CalculateImageSpaceUsage returns the total space taken up by docker images on a specified host
	CalculateImageSpaceUsage(ctx context.Context, h *host.Host) (int64, error)
	// GetContainerImage downloads a container image onto parent specified by URL, and builds evergreen agent unless otherwise specified
	GetContainerImage(ctx context.Context, parent *host.Host, options host.DockerOptions) error
}

// BatchManager is an interface for cloud providers that support batch operations.
type BatchManager interface {
	// GetInstanceStatuses gets the statuses of a slice of instances. It returns
	// a map of the instance IDs to their current status. If some of the
	// instance statuses cannot be retrieved, implementations are allowed to
	// either return an error or return StatusNonExistent for those hosts.
	// If there is no error, implementations should return the same number of
	// results in the map as there are hosts.
	GetInstanceStatuses(context.Context, []host.Host) (map[string]CloudStatus, error)
}

// ManagerOpts is a struct containing the fields needed to get a new cloud manager
// of the proper type.
type ManagerOpts struct {
	Provider string
	// Account is the account where API calls are being made.
	Account string
	// Region is the geographical region where the cloud operations should
	// occur.
	Region string
}

// GetSettings returns an uninitialized ProviderSettings based on the given
// provider.
func GetSettings(provider string) (ProviderSettings, error) {
	switch provider {
	case evergreen.ProviderNameEc2OnDemand, evergreen.ProviderNameEc2Fleet:
		return &EC2ProviderSettings{}, nil
	case evergreen.ProviderNameStatic:
		return &StaticSettings{}, nil
	case evergreen.ProviderNameMock:
		return &MockProviderSettings{}, nil
	case evergreen.ProviderNameDocker, evergreen.ProviderNameDockerMock:
		return &dockerSettings{}, nil
	}
	return nil, errors.Errorf("invalid provider name '%s'", provider)
}

// GetManager returns an implementation of Manager for the given manager options.
// It returns an error if the provider name doesn't have a known implementation.
func GetManager(ctx context.Context, env evergreen.Environment, mgrOpts ManagerOpts) (Manager, error) {
	var provider Manager

	switch mgrOpts.Provider {
	case evergreen.ProviderNameEc2OnDemand:
		provider = &ec2Manager{
			env: env,
			EC2ManagerOptions: &EC2ManagerOptions{
				client:  &awsClientImpl{},
				account: mgrOpts.Account,
				region:  mgrOpts.Region,
			},
		}
	case evergreen.ProviderNameEc2Fleet:
		provider = &ec2FleetManager{
			env: env,
			EC2FleetManagerOptions: &EC2FleetManagerOptions{
				client:  &awsClientImpl{},
				account: mgrOpts.Account,
				region:  mgrOpts.Region,
			},
		}
	case evergreen.ProviderNameStatic:
		provider = &staticManager{}
	case evergreen.ProviderNameMock:
		provider = makeMockManager()
	case evergreen.ProviderNameDocker:
		provider = &dockerManager{env: env}
	case evergreen.ProviderNameDockerMock:
		provider = &dockerManager{env: env, client: &dockerClientMock{}}
	default:
		return nil, errors.Errorf("no known provider '%s'", mgrOpts.Provider)
	}

	if err := provider.Configure(ctx, env.Settings()); err != nil {
		return nil, errors.Wrap(err, "configuring cloud provider")
	}

	return provider, nil
}

// GetManagerOptions gets the manager options from the provider settings object for a given
// provider name.
func GetManagerOptions(d distro.Distro) (ManagerOpts, error) {
	region := ""
	if len(d.ProviderSettingsList) > 1 {
		if evergreen.IsEc2Provider(d.Provider) {
			// this shouldn't ever happen, but if it does we want to continue so we don't inadvertently block task queues
			grip.Error(message.Fields{
				"message":           "distro should be modified to have only one provider settings",
				"provider_settings": d.ProviderSettingsList,
				"distro":            d.Id,
			})
			region = evergreen.DefaultEC2Region
		} else {
			return ManagerOpts{}, errors.Errorf(
				"distro '%s' has multiple provider settings, but is provider '%s'", d.Id, d.Provider)
		}
	}
	if evergreen.IsEc2Provider(d.Provider) {
		s := &EC2ProviderSettings{}
		if err := s.FromDistroSettings(d, region); err != nil {
			return ManagerOpts{}, errors.Wrapf(err, "getting EC2 provider settings from distro")
		}

		return getEC2ManagerOptionsFromSettings(d, s), nil
	}
	if d.Provider == evergreen.ProviderNameMock {
		return getMockManagerOptions(d)
	}
	return ManagerOpts{Provider: d.Provider}, nil
}

// ConvertContainerManager converts a regular manager into a container manager,
// errors if type conversion not possible.
func ConvertContainerManager(m Manager) (ContainerManager, error) {
	if cm, ok := m.(ContainerManager); ok {
		return cm, nil
	}
	return nil, errors.Errorf("programmer error: cannot convert manager %T to container manager", m)
}

// VolumeAttachment contains information about a volume attached to a host.
type VolumeAttachment struct {
	VolumeID   string
	HostID     string
	DeviceName string
}
