package mock

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
)

const ProviderName = "mock"

type MockCloudManager struct{}

func (staticMgr *MockCloudManager) SpawnInstance(distro *model.Distro, owner string, userHost bool) (*model.Host, error) {
	return &model.Host{
		Id:        util.RandomString(),
		Distro:    distro.Name,
		StartedBy: owner,
	}, nil
}

// get the status of an instance
func (staticMgr *MockCloudManager) GetInstanceStatus(host *model.Host) (cloud.CloudStatus, error) {
	return cloud.StatusRunning, nil
}

// get instance DNS
func (staticMgr *MockCloudManager) GetDNSName(host *model.Host) (string, error) {
	return host.Host, nil
}

func (staticMgr *MockCloudManager) CanSpawn() (bool, error) {
	return true, nil
}

// terminate an instance
func (staticMgr *MockCloudManager) TerminateInstance(host *model.Host) error {
	return nil
}

func (staticMgr *MockCloudManager) Configure(mciSettings *mci.MCISettings) error {
	//no-op. maybe will need to load something from mciSettings in the future.
	return nil
}

func (staticMgr *MockCloudManager) IsSSHReachable(host *model.Host, distro *model.Distro,
	keyPath string) (bool, error) {
	return true, nil
}

func (staticMgr *MockCloudManager) IsUp(host *model.Host) (bool, error) {
	return true, nil
}

func (staticMsg *MockCloudManager) OnUp(host *model.Host) error {
	return nil
}

func (staticMgr *MockCloudManager) GetSSHOptions(host *model.Host, distro *model.Distro,
	keyPath string) ([]string, error) {
	return []string{}, nil
}
