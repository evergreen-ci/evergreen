package cloud

import (
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
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
	Status             CloudStatus
	SSHOptions         []string
	TimeTilNextPayment time.Duration
	DNSName            string
	OnUpRan            bool
}

var mockInstances map[string]MockInstance

func init() {
	ResetMock()
}

func ResetMock() {
	mockInstances = map[string]MockInstance{}
}

// mockManager implements the CloudManager interface for testing
// purposes. It contains a map of MockInstances that it knows about
// which its various functions return information about. Once set before
// testing, this map should only be touched either through the associated
// cloud manager functions, or in association with the mutex.
type mockManager struct {
	Instances map[string]MockInstance
	mutex     *sync.RWMutex
}

func makeMockManager() CloudManager {
	return &mockManager{
		Instances: mockInstances,
		mutex:     &lock,
	}
}

func (mockMgr *mockManager) SpawnHost(h *host.Host) (*host.Host, error) {
	l := mockMgr.mutex
	l.Lock()
	defer l.Unlock()
	mockMgr.Instances[h.Id] = MockInstance{
		IsUp:               false,
		IsSSHReachable:     false,
		Status:             StatusInitializing,
		SSHOptions:         []string{},
		TimeTilNextPayment: time.Duration(0),
		DNSName:            "",
	}
	return h, nil
}

// get the status of an instance
func (mockMgr *mockManager) GetInstanceStatus(host *host.Host) (CloudStatus, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return StatusUnknown, errors.Errorf("unable to fetch host: %s", host.Id)
	}

	return instance.Status, nil
}

// get instance DNS
func (mockMgr *mockManager) GetDNSName(host *host.Host) (string, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return "", errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.DNSName, nil
}

func (_ *mockManager) GetSettings() ProviderSettings {
	return &mockManager{}
}

func (_ *mockManager) Validate() error {
	return nil
}

func (mockMgr *mockManager) CanSpawn() (bool, error) {
	return true, nil
}

func (*mockManager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

// terminate an instance
func (mockMgr *mockManager) TerminateInstance(host *host.Host) error {
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

	instance.Status = StatusTerminated
	mockMgr.Instances[host.Id] = instance

	return errors.WithStack(host.Terminate())
}

func (mockMgr *mockManager) Configure(settings *evergreen.Settings) error {
	//no-op. maybe will need to load something from settings in the future.
	return nil
}

func (mockMgr *mockManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return false, errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.IsSSHReachable, nil
}

func (mockMgr *mockManager) IsUp(host *host.Host) (bool, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return false, errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.IsUp, nil
}

func (mockMgr *mockManager) OnUp(host *host.Host) error {
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

func (mockMgr *mockManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return []string{}, errors.Errorf("unable to fetch host: %s", host.Id)
	}
	return instance.SSHOptions, nil
}

func (mockMgr *mockManager) TimeTilNextPayment(host *host.Host) time.Duration {
	l := mockMgr.mutex
	l.RLock()
	instance, ok := mockMgr.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return time.Duration(0)
	}
	return instance.TimeTilNextPayment
}
