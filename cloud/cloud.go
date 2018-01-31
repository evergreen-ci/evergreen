package cloud

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// ProviderSettings exposes provider-specific configuration settings for a CloudManager.
type ProviderSettings interface {
	Validate() error
}

//CloudManager is an interface which handles creating new hosts or modifying
//them via some third-party API.
type CloudManager interface {
	// Returns a pointer to the manager's configuration settings struct
	GetSettings() ProviderSettings

	//Load credentials or other settings from the config file
	Configure(*evergreen.Settings) error

	// SpawnHost attempts to create a new host by requesting one from the
	// provider's API.
	SpawnHost(h *host.Host) (*host.Host, error)

	// CanSpawn indicates if this provider is capable of creating new instances
	// with SpawnInstance(). If this provider doesn't support spawning new
	// hosts, this will return false (and calls to SpawnInstance will
	// return errors)
	CanSpawn() (bool, error)

	// get the status of an instance
	GetInstanceStatus(*host.Host) (CloudStatus, error)

	// TerminateInstances destroys the host in the underlying provider
	TerminateInstance(*host.Host, string) error

	//IsUp returns true if the underlying provider has not destroyed the
	//host (in other words, if the host "should" be reachable. This does not
	//necessarily mean that the host actually *is* reachable via SSH
	IsUp(*host.Host) (bool, error)

	//Called by the hostinit process when the host is actually up. Used
	//to set additional provider-specific metadata
	OnUp(*host.Host) error

	//IsSSHReachable returns true if the host can successfully
	//accept and run an ssh command.
	IsSSHReachable(host *host.Host, keyPath string) (bool, error)

	// GetDNSName returns the DNS name of a host.
	GetDNSName(*host.Host) (string, error)

	// GetSSHOptions generates the command line args to be passed to ssh to
	// allow connection to the machine
	GetSSHOptions(host *host.Host, keyName string) ([]string, error)

	// TimeTilNextPayment returns how long there is until the next payment
	// is due for a particular host
	TimeTilNextPayment(host *host.Host) time.Duration

	// GetInstanceName returns the name that should be used for an instance of this provider
	GetInstanceName(d *distro.Distro) string
}

// CloudCostCalculator is an interface for cloud managers that can estimate an
// what a span of time on a given host costs.
type CloudCostCalculator interface {
	CostForDuration(host *host.Host, start time.Time, end time.Time) (float64, error)
}

// GetCloudManager returns an implementation of CloudManager for the given provider name.
// It returns an error if the provider name doesn't have a known implementation.
func GetCloudManager(providerName string, settings *evergreen.Settings) (CloudManager, error) {
	var provider CloudManager

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
	case evergreen.ProviderNameOpenstack:
		provider = &openStackManager{}
	case evergreen.ProviderNameGce:
		provider = &gceManager{}
	case evergreen.ProviderNameVsphere:
		provider = &vsphereManager{}
	default:
		return nil, errors.Errorf("No known provider for '%v'", providerName)
	}

	if err := provider.Configure(settings); err != nil {
		return nil, errors.Wrap(err, "Failed to configure cloud provider")
	}

	return provider, nil
}
