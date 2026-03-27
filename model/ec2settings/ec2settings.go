// Package ec2settings resolves EC2 distro provider settings by region for model code that cannot import cloud.
package ec2settings

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/ec2mount"
)

// MountPointsForDistro returns mount points from the distro's EC2 provider settings for the given region.
// It selects settings by region (falling back to evergreen.DefaultEC2Region if empty or not found, then to
// the first region) to avoid over-counting. It returns nil when the provider is not EC2, when there are no
// provider settings, or when mount points are missing or cannot be decoded.
func MountPointsForDistro(d *distro.Distro, region string) ([]ec2mount.MountPoint, error) {
	if d == nil {
		return nil, nil
	}
	if d.Provider != evergreen.ProviderNameEc2OnDemand && d.Provider != evergreen.ProviderNameEc2Fleet {
		return nil, nil
	}
	if len(d.ProviderSettingsList) == 0 {
		return nil, nil
	}
	if region == "" {
		region = evergreen.DefaultEC2Region
	}
	settingsDoc, err := d.GetProviderSettingByRegion(region)
	if err != nil {
		settingsDoc, err = d.GetProviderSettingByRegion(evergreen.DefaultEC2Region)
		if err != nil {
			settingsDoc = d.ProviderSettingsList[0]
		}
	}
	mountPoints, err := ec2mount.MountPointsFromProviderDocument(settingsDoc)
	if err != nil {
		return nil, nil
	}
	if len(mountPoints) == 0 {
		return nil, nil
	}
	return mountPoints, nil
}
