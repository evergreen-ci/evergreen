package mock

import (
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// MockInstance mocks a running server that Evergreen knows about. It contains
// fields that can be set to change the response the cloud manager returns
// when this mock instance is queried for.
type MockInstance struct {
	IsUp               bool
	IsSSHReachable     bool
	Status             cloud.CloudStatus
	SSHOptions         []string
	TimeTilNextPayment time.Duration
	DNSName            string
	OnUpRan            bool
}

var MockInstances map[string]MockInstance = map[string]MockInstance{}
var lock = sync.RWMutex{}

func Clear() {
	MockInstances = map[string]MockInstance{}
	lock = sync.RWMutex{}
}

// MockCloudManager implements the CloudManager interface for testing
// purposes. It contains a map of MockInstances that it knows about
// which its various functions return information about. Once set before
// testing, this map should only be touched either through the associated
// cloud manager functions, or in association with the mutex.
type MockCloudManager struct {
	Instances map[string]MockInstance
	mutex     *sync.RWMutex
}

func FetchMockProvider() *MockCloudManager {
	return &MockCloudManager{
		Instances: MockInstances,
		mutex:     &lock,
	}
}

func (mockMgr *MockCloudManager) SpawnHost(h *host.Host) (*host.Host, error) {
	l := mockMgr.mutex
	l.Lock()
	defer l.Unlock()
	mockMgr.Instances[h.Id] = MockInstance{
		IsUp:               false,
		IsSSHReachable:     false,
		Status:             cloud.StatusInitializing,
		SSHOptions:         []string{},
		TimeTilNextPayment: time.Duration(0),
		DNSName:            "",
	}
	return h, nil
}

// get the status of an instance
func (mockMgr *MockCloudManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return cloud.StatusUnknown, errors.Errorf("unable to fetch host: %s", host.Id)
	}

	return instance.Status, nil
}

// get instance DNS
func (mockMgr *MockCloudManager) GetDNSName(host *host.Host) (string, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return "", errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.DNSName, nil
}

func (_ *MockCloudManager) GetSettings() cloud.ProviderSettings {
	return &MockCloudManager{}
}

func (_ *MockCloudManager) Validate() error {
	return nil
}

func (mockMgr *MockCloudManager) CanSpawn() (bool, error) {
	return true, nil
}

func (*MockCloudManager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

// terminate an instance
func (mockMgr *MockCloudManager) TerminateInstance(host *host.Host) error {
	l := mockMgr.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := mockMgr.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host: %s", host.Id)
	}
	if host.Status == evergreen.HostTerminated {
		return errors.Errorf("Cannot terminate %s; already marked as terminated!", host.Id)
	}

	instance.Status = cloud.StatusTerminated
	mockMgr.Instances[host.Id] = instance

	return errors.WithStack(host.Terminate())
}

func (mockMgr *MockCloudManager) Configure(settings *evergreen.Settings) error {
	//no-op. maybe will need to load something from settings in the future.
	return nil
}

func (mockMgr *MockCloudManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return false, errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.IsSSHReachable, nil
}

func (mockMgr *MockCloudManager) IsUp(host *host.Host) (bool, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return false, errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.IsUp, nil
}

func (mockMgr *MockCloudManager) OnUp(host *host.Host) error {
	l := mockMgr.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := mockMgr.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host: %s", host.Id)
	}
	instance.OnUpRan = true
	mockMgr.Instances[host.Id] = instance

	return nil
}

func (mockMgr *MockCloudManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return []string{}, errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.SSHOptions, nil
}

func (mockMgr *MockCloudManager) TimeTilNextPayment(host *host.Host) time.Duration {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return time.Duration(0)
	}
	return instance.TimeTilNextPayment
}
