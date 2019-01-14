package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
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

	//Load credentials or other settings from the config file
	Configure(context.Context, *evergreen.Settings) error

	// SpawnHost attempts to create a new host by requesting one from the
	// provider's API.
	SpawnHost(context.Context, *host.Host) (*host.Host, error)

	// get the status of an instance
	GetInstanceStatus(context.Context, *host.Host) (CloudStatus, error)

	// TerminateInstances destroys the host in the underlying provider
	TerminateInstance(context.Context, *host.Host, string) error

	//IsUp returns true if the underlying provider has not destroyed the
	//host (in other words, if the host "should" be reachable. This does not
	//necessarily mean that the host actually *is* reachable via SSH
	IsUp(context.Context, *host.Host) (bool, error)

	//Called by the hostinit process when the host is actually up. Used
	//to set additional provider-specific metadata
	OnUp(context.Context, *host.Host) error

	// GetDNSName returns the DNS name of a host.
	GetDNSName(context.Context, *host.Host) (string, error)

	// GetSSHOptions generates the command line args to be passed to ssh to
	// allow connection to the machine
	GetSSHOptions(*host.Host, string) ([]string, error)

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
	// BuildContainerImage downloads and builds a container image onto parent specified by URL
	BuildContainerImage(ctx context.Context, parent *host.Host, settings ContainerImageSettings) error
}

// CostCalculator is an interface for cloud providers that can estimate what a span of time on a
// given host costs.
type CostCalculator interface {
	CostForDuration(context.Context, *host.Host, time.Time, time.Time, *evergreen.Settings) (float64, error)
}

// BatchManager is an interface for cloud providers that support batch operations.
type BatchManager interface {
	// GetInstanceStatuses gets the status of a slice of instances.
	GetInstanceStatuses(context.Context, []host.Host) ([]CloudStatus, error)
}

// GetManager returns an implementation of Manager for the given provider name.
// It returns an error if the provider name doesn't have a known implementation.
func GetManager(ctx context.Context, providerName string, settings *evergreen.Settings) (Manager, error) {
	var provider Manager

	switch providerName {
	case evergreen.ProviderNameStatic:
		provider = &staticManager{}
	case evergreen.ProviderNameMock:
		provider = makeMockManager()
	case evergreen.ProviderNameEc2Legacy, evergreen.ProviderNameEc2OnDemand:
		provider = NewEC2Manager(&EC2ManagerOptions{client: &awsClientImpl{}, provider: onDemandProvider})
	case evergreen.ProviderNameEc2Spot:
		provider = NewEC2Manager(&EC2ManagerOptions{client: &awsClientImpl{}, provider: spotProvider})
	case evergreen.ProviderNameEc2Auto:
		provider = NewEC2Manager(&EC2ManagerOptions{client: &awsClientImpl{}, provider: autoProvider})
	case evergreen.ProviderNameDocker:
		provider = &dockerManager{}
	case evergreen.ProviderNameDockerMock:
		provider = &dockerManager{client: &dockerClientMock{}}
	case evergreen.ProviderNameOpenstack:
		provider = &openStackManager{}
	case evergreen.ProviderNameGce:
		provider = &gceManager{}
	case evergreen.ProviderNameVsphere:
		provider = &vsphereManager{}
	default:
		return nil, errors.Errorf("No known provider for '%s'", providerName)
	}

	if err := provider.Configure(ctx, settings); err != nil {
		return nil, errors.Wrap(err, "Failed to configure cloud provider")
	}

	return provider, nil
}

// ConvertContainerManager converts a regular manager into a container manager,
// errors if type conversion not possible.
func ConvertContainerManager(m Manager) (ContainerManager, error) {
	if cm, ok := m.(ContainerManager); ok {
		return cm, nil
	}
	return nil, errors.New("Error converting manager to container manager")
}
