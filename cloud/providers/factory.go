package providers

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers/digitalocean"
	"github.com/evergreen-ci/evergreen/cloud/providers/docker"
	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/cloud/providers/static"
	"github.com/evergreen-ci/evergreen/model/host"
)

// GetCloudManager returns an implementation of CloudManager for the given provider name.
// It returns an error if the provider name doesn't have a known implementation.
func GetCloudManager(providerName string, settings *evergreen.Settings) (cloud.CloudManager, error) {

	var provider cloud.CloudManager
	switch providerName {
	case static.ProviderName:
		provider = &static.StaticManager{}
	case mock.ProviderName:
		provider = &mock.MockCloudManager{}
	case digitalocean.ProviderName:
		provider = &digitalocean.DigitalOceanManager{}
	case ec2.OnDemandProviderName:
		provider = &ec2.EC2Manager{}
	case ec2.SpotProviderName:
		provider = &ec2.EC2SpotManager{}
	case docker.ProviderName:
		provider = &docker.DockerManager{}
	default:
		return nil, fmt.Errorf("No known provider for '%v'", providerName)
	}

	if err := provider.Configure(settings); err != nil {
		return nil, fmt.Errorf("Failed to configure ec2 provider: %v", err)
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
