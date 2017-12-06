package providers

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers/digitalocean"
	"github.com/evergreen-ci/evergreen/cloud/providers/docker"
	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/cloud/providers/gce"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/cloud/providers/openstack"
	"github.com/evergreen-ci/evergreen/cloud/providers/static"
	"github.com/evergreen-ci/evergreen/cloud/providers/vsphere"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// GetCloudManager returns an implementation of CloudManager for the given provider name.
// It returns an error if the provider name doesn't have a known implementation.
func GetCloudManager(providerName string, settings *evergreen.Settings) (cloud.CloudManager, error) {

	var provider cloud.CloudManager
	switch providerName {
	case evergreen.ProviderNameStatic:
		provider = &static.StaticManager{}
	case evergreen.ProviderNameMock:
		provider = mock.FetchMockProvider()
	case evergreen.ProviderNameDigitalOcean:
		provider = &digitalocean.DigitalOceanManager{}
	case evergreen.ProviderNameEc2OnDemand:
		provider = &ec2.EC2Manager{}
	case evergreen.ProviderNameEc2Spot:
		provider = &ec2.EC2SpotManager{}
	case evergreen.ProviderNameDocker:
		provider = &docker.Manager{}
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
func GetCloudHost(host *host.Host, settings *evergreen.Settings) (*cloud.CloudHost, error) {
	mgr, err := GetCloudManager(host.Provider, settings)
	if err != nil {
		return nil, err
	}

	keyPath := ""
	if host.Distro.SSHKey != "" {
		keyPath = settings.Keys[host.Distro.SSHKey]
	}
	return &cloud.CloudHost{host, keyPath, mgr}, nil
}
