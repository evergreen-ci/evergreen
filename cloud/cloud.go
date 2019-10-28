package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// ProviderSettings exposes provider-specific configuration settings for a Manager.
type ProviderSettings interface {
	Validate() error
}

//Manager is an interface which handles creating new hosts or modifying
//them via some third-party API.
type Manager interface {
	// Returns a pointer to the manager's configuration settings struct
	GetSettings() ProviderSettings

	// Load credentials or other settings from the config file
	Configure(context.Context, *evergreen.Settings) error

	// SpawnHost attempts to create a new host by requesting one from the
	// provider's API.
	SpawnHost(context.Context, *host.Host) (*host.Host, error)

	// ModifyHost modifies an existing host
	ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error

	// Get the status of an instance
	GetInstanceStatus(context.Context, *host.Host) (CloudStatus, error)

	// TerminateInstance destroys the host in the underlying provider
	TerminateInstance(context.Context, *host.Host, string, string) error

	// StopInstance stops an instance.
	StopInstance(context.Context, *host.Host, string) error

	// StartInstance starts a stopped instance.
	StartInstance(context.Context, *host.Host, string) error

	// IsUp returns true if the underlying provider has not destroyed the
	// host (in other words, if the host "should" be reachable. This does not
	// necessarily mean that the host actually *is* reachable via SSH
	IsUp(context.Context, *host.Host) (bool, error)

	// Called by the hostinit process when the host is actually up. Used
	// to set additional provider-specific metadata
	OnUp(context.Context, *host.Host) error

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

	// TimeTilNextPayment returns how long there is until the next payment
	// is due for a particular host
	TimeTilNextPayment(*host.Host) time.Duration
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

// CostCalculator is an interface for cloud providers that can estimate what a span of time on a
// given host costs.
type CostCalculator interface {
	CostForDuration(context.Context, *host.Host, time.Time, time.Time) (float64, error)
}

// BatchManager is an interface for cloud providers that support batch operations.
type BatchManager interface {
	// GetInstanceStatuses gets the status of a slice of instances.
	GetInstanceStatuses(context.Context, []host.Host) ([]CloudStatus, error)
}

// ManagerOpts is a struct containing the fields needed to get a new cloud manager
// of the proper type.
type ManagerOpts struct {
	Provider string
	Region   string
}

// GetManager returns an implementation of Manager for the given manager options.
// It returns an error if the provider name doesn't have a known implementation.
func GetManager(ctx context.Context, env evergreen.Environment, mgrOpts ManagerOpts) (Manager, error) {
	var provider Manager

	switch mgrOpts.Provider {
	case evergreen.ProviderNameStatic:
		provider = &staticManager{}
	case evergreen.ProviderNameMock:
		provider = makeMockManager()
	case evergreen.ProviderNameEc2OnDemand:
		provider = &ec2Manager{env: env, EC2ManagerOptions: &EC2ManagerOptions{client: &awsClientImpl{}, provider: onDemandProvider, region: mgrOpts.Region}}
	case evergreen.ProviderNameEc2Spot:
		provider = &ec2Manager{env: env, EC2ManagerOptions: &EC2ManagerOptions{client: &awsClientImpl{}, provider: spotProvider, region: mgrOpts.Region}}
	case evergreen.ProviderNameEc2Auto:
		provider = &ec2Manager{env: env, EC2ManagerOptions: &EC2ManagerOptions{client: &awsClientImpl{}, provider: autoProvider, region: mgrOpts.Region}}
	case evergreen.ProviderNameEc2Fleet:
		provider = &ec2FleetManager{env: env, EC2FleetManagerOptions: &EC2FleetManagerOptions{client: &awsClientImpl{}, region: mgrOpts.Region}}
	case evergreen.ProviderNameDocker:
		provider = &dockerManager{env: env}
	case evergreen.ProviderNameDockerMock:
		provider = &dockerManager{env: env, client: &dockerClientMock{}}
	case evergreen.ProviderNameOpenstack:
		provider = &openStackManager{}
	case evergreen.ProviderNameGce:
		provider = &gceManager{}
	case evergreen.ProviderNameVsphere:
		provider = &vsphereManager{}
	default:
		return nil, errors.Errorf("No known provider for '%s'", mgrOpts.Provider)
	}

	if err := provider.Configure(ctx, env.Settings()); err != nil {
		return nil, errors.Wrap(err, "Failed to configure cloud provider")
	}

	return provider, nil
}

// GroupHostsByManager returns a map where the key represents the manager options
// (provider name and region) and the value represents the hosts from the provided
// slice that are reachable through that manager.
func GroupHostsByManager(hosts []host.Host) map[ManagerOpts][]host.Host {
	hostsByManager := make(map[ManagerOpts][]host.Host)
	for _, h := range hosts {
		key := ManagerOpts{
			Provider: h.Provider,
			Region:   GetRegion(h.Distro),
		}
		hostsByManager[key] = append(hostsByManager[key], h)
	}
	return hostsByManager
}

// GetRegion gets the region name from the provider settings object for a given
// provider name.
func GetRegion(d distro.Distro) string {
	if IsEc2Provider(d.Provider) {
		return getEC2Region(d.ProviderSettings)
	}
	if d.Provider == evergreen.ProviderNameMock {
		return getMockRegion(d.ProviderSettings)
	}
	return ""
}

// ConvertContainerManager converts a regular manager into a container manager,
// errors if type conversion not possible.
func ConvertContainerManager(m Manager) (ContainerManager, error) {
	if cm, ok := m.(ContainerManager); ok {
		return cm, nil
	}
	return nil, errors.New("Error converting manager to container manager")
}
