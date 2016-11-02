package mock

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
)

const ProviderName = "mock"

type MockCloudManager struct{}

func (staticMgr *MockCloudManager) SpawnInstance(distro *distro.Distro, hostOpts cloud.HostOptions) (*host.Host, error) {
	return &host.Host{
		Id:        util.RandomString(),
		Distro:    *distro,
		StartedBy: hostOpts.UserName,
	}, nil
}

// get the status of an instance
func (staticMgr *MockCloudManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	return cloud.StatusRunning, nil
}

// get instance DNS
func (staticMgr *MockCloudManager) GetDNSName(host *host.Host) (string, error) {
	return host.Host, nil
}

func (_ *MockCloudManager) GetSettings() cloud.ProviderSettings {
	return &MockCloudManager{}
}

func (_ *MockCloudManager) Validate() error {
	return nil
}

func (staticMgr *MockCloudManager) CanSpawn() (bool, error) {
	return true, nil
}

// terminate an instance
func (staticMgr *MockCloudManager) TerminateInstance(host *host.Host) error {
	return nil
}

func (staticMgr *MockCloudManager) Configure(settings *evergreen.Settings) error {
	//no-op. maybe will need to load something from settings in the future.
	return nil
}

func (staticMgr *MockCloudManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	return true, nil
}

func (staticMgr *MockCloudManager) IsUp(host *host.Host) (bool, error) {
	return true, nil
}

func (staticMsg *MockCloudManager) OnUp(host *host.Host) error {
	return nil
}

func (staticMgr *MockCloudManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	return []string{}, nil
}

func (staticMgr *MockCloudManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
