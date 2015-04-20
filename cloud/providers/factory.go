package providers

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/cloud/providers/digitalocean"
	"10gen.com/mci/cloud/providers/ec2"
	"10gen.com/mci/cloud/providers/mock"
	"10gen.com/mci/cloud/providers/static"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"fmt"
)

// GetCloudManager returns an implementation of CloudManager for the given provider name.
// It returns an error if the provider name doesn't have a known implementation.
func GetCloudManager(providerName string, mciSettings *mci.MCISettings) (cloud.CloudManager, error) {

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
	default:
		return nil, fmt.Errorf("No known provider for: %v", providerName)
	}

	if err := provider.Configure(mciSettings); err != nil {
		return nil, fmt.Errorf("Failed to configure ec2 provider: %v", err)
	}

	return provider, nil
}

// GetCloudHost returns an instance of CloudHost wrapping the given model.Host,
// giving access to the provider-specific methods to manipulate on the host.
func GetCloudHost(host *host.Host, mciSettings *mci.MCISettings) (*cloud.CloudHost, error) {
	mgr, err := GetCloudManager(host.Provider, mciSettings)
	if err != nil {
		return nil, err
	}

	distro, err := distro.LoadOne(mciSettings.ConfigDir, host.Distro)
	if err != nil {
		return nil, fmt.Errorf("Failed to load distro '%v' for host '%v': %v", host.Distro, host.Id, err)
	}
	if distro == nil {
		return nil, fmt.Errorf("Distro '%v' not found for host '%v'", host.Distro, host.Id)
	}

	keyPath := ""
	if distro.Key != "" {
		keyPath = mciSettings.Keys[distro.Key]
	}
	return &cloud.CloudHost{host, distro, keyPath, mgr}, nil
}
