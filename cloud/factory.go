package cloud

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/gce"
	"github.com/evergreen-ci/evergreen/cloud/providers/openstack"
	"github.com/evergreen-ci/evergreen/cloud/providers/vsphere"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// GetCloudManager returns an implementation of CloudManager for the given provider name.
// It returns an error if the provider name doesn't have a known implementation.
func GetCloudManager(providerName string, settings *evergreen.Settings) (CloudManager, error) {
	var provider CloudManager

	switch providerName {
	case evergreen.ProviderNameStatic:
		provider = &staticManager{}
	case evergreen.ProviderNameMock:
		provider = FetchMockProvider()
	case evergreen.ProviderNameDigitalOcean:
		provider = &doManager{}
	case evergreen.ProviderNameEc2OnDemand:
		provider = &ec2OnDemandManager{}
	case evergreen.ProviderNameEc2Spot:
		provider = &ec2SpotManager{}
	case evergreen.ProviderNameDocker:
		provider = &dockerManager{}
	case evergreen.ProviderNameOpenstack:
		provider = &openstack.Manager{}
	case evergreen.ProviderNameGce:
		provider = &gce.Manager{}
	case evergreen.ProviderNameVsphere:
		provider = &vsphere.Manager{}
	default:
		return nil, errors.Errorf("No known provider for '%v'", providerName)
	}

	if err := provider.Configure(settings); err != nil {
		return nil, errors.Wrap(err, "Failed to configure cloud provider")
	}

	return provider, nil
}

// GetCloudHost returns an instance of CloudHost wrapping the given model.Host,
// giving access to the provider-specific methods to manipulate on the host.
func GetCloudHost(host *host.Host, settings *evergreen.Settings) (*CloudHost, error) {
	mgr, err := GetCloudManager(host.Provider, settings)
	if err != nil {
		return nil, err
	}

	keyPath := ""
	if host.Distro.SSHKey != "" {
		keyPath = settings.Keys[host.Distro.SSHKey]
	}
	return &CloudHost{host, keyPath, mgr}, nil
}
