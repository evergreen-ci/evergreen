package cloud

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var globalMockState *mockState

func init() {
	globalMockState = &mockState{
		instances: map[string]MockInstance{},
	}
}

// MockInstance mocks a running server that Evergreen knows about. It contains
// fields that can be set to change the response the cloud manager returns
// when this mock instance is queried for.
type MockInstance struct {
	IsSSHReachable     bool
	Status             CloudStatus
	SSHOptions         []string
	TimeTilNextPayment time.Duration
	DNSName            string
	Tags               []host.Tag
	Type               string
	BlockDevices       []string
}

type MockVolume struct {
	DeviceName   string
	Type         string
	Size         int32
	Expiration   time.Time
	NoExpiration bool
}

type MockProvider interface {
	Len() int
	Reset()
	Get(string) MockInstance
	Set(string, MockInstance)
	IterIDs() <-chan string
	IterInstances() <-chan MockInstance
}

type MockProviderSettings struct {
	Region string `mapstructure:"region" json:"region" bson:"region,omitempty"`
}

func GetMockProvider() MockProvider {
	return globalMockState
}

func (_ *MockProviderSettings) Validate() error {
	return nil
}

func (_ *MockProviderSettings) FromDistroSettings(_ distro.Distro, _ string) error {
	return nil
}

type mockState struct {
	instances map[string]MockInstance
	mutex     sync.RWMutex
}

func (m *mockState) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.instances = map[string]MockInstance{}
}

func (m *mockState) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.instances)
}

func (m *mockState) IterIDs() <-chan string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	out := make(chan string, len(m.instances))

	for id := range m.instances {
		out <- id
	}

	close(out)
	return out
}

func (m *mockState) Get(id string) MockInstance {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.instances[id]
}

func (m *mockState) Set(id string, instance MockInstance) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.instances[id] = instance
}

func (m *mockState) IterInstances() <-chan MockInstance {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	out := make(chan MockInstance, len(m.instances))

	for _, node := range m.instances {
		out <- node
	}

	close(out)
	return out
}

// mockManager implements the Manager interface for testing
// purposes. It contains a map of MockInstances that it knows about
// which its various functions return information about. Once set before
// testing, this map should only be touched either through the associated
// cloud manager functions, or in association with the mutex.
type mockManager struct {
	Instances map[string]MockInstance
	Volumes   map[string]MockVolume
	mutex     *sync.RWMutex
}

func makeMockManager() Manager {
	return &mockManager{
		Instances: globalMockState.instances,
		mutex:     &globalMockState.mutex,
	}
}

func (m *mockManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	m.Instances[h.Id] = MockInstance{
		IsSSHReachable:     false,
		Status:             StatusInitializing,
		SSHOptions:         []string{},
		TimeTilNextPayment: time.Duration(0),
		DNSName:            "",
	}
	return h, nil
}

func (m *mockManager) ModifyHost(ctx context.Context, host *host.Host, changes host.HostModifyOptions) error {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	var err error
	instance, ok := m.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", host.Id)
	}

	if len(changes.AddInstanceTags) > 0 {
		host.AddTags(changes.AddInstanceTags)
		instance.Tags = host.InstanceTags
		m.Instances[host.Id] = instance
		if err = host.SetTags(ctx); err != nil {
			return errors.Errorf("adding tags in DB")
		}
	}

	if len(changes.DeleteInstanceTags) > 0 {
		instance.Tags = host.InstanceTags
		m.Instances[host.Id] = instance
		host.DeleteTags(changes.DeleteInstanceTags)
		if err = host.SetTags(ctx); err != nil {
			return errors.Errorf("deleting tags in DB")
		}
	}

	if changes.InstanceType != "" {
		instance.Type = host.InstanceType
		m.Instances[host.Id] = instance
		if err = host.SetInstanceType(ctx, changes.InstanceType); err != nil {
			return errors.Errorf("setting instance type in DB")
		}
	}

	if changes.NoExpiration != nil {
		expireOnValue := expireInDays(30)
		if *changes.NoExpiration {
			var userTimeZone string
			u, err := user.FindOneByIdContext(ctx, host.StartedBy)
			if err != nil {
				return errors.Wrapf(err, "finding owner '%s' for host '%s'", host.StartedBy, host.Id)
			}
			if u != nil {
				userTimeZone = u.Settings.Timezone
			}
			if err = host.MarkShouldNotExpire(ctx, expireOnValue, userTimeZone); err != nil {
				return errors.Errorf("setting no expiration in DB")
			}
		} else {
			if err = host.MarkShouldExpire(ctx, expireOnValue); err != nil {
				return errors.Errorf("setting expiration in DB")
			}
		}
	}

	if changes.NewName != "" {
		if err = host.SetDisplayName(ctx, changes.NewName); err != nil {
			return errors.Errorf("setting display name in DB")
		}
	}

	return nil
}

// GetInstanceStatus gets the state of one instance according to the instance
// data stored in the mock manager.
func (m *mockManager) GetInstanceState(ctx context.Context, h *host.Host) (CloudInstanceState, error) {
	info := CloudInstanceState{}
	info.Status = m.getOrDefaultInstanceStatus(ctx, h.Id)
	return info, nil
}

func (m *mockManager) getOrDefaultInstanceStatus(ctx context.Context, hostID string) CloudStatus {
	m.mutex.RLock()
	instance, ok := m.Instances[hostID]
	m.mutex.RUnlock()
	if !ok {
		return StatusNonExistent
	}
	return instance.Status
}

func (m *mockManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return nil
}

// get instance DNS
func (m *mockManager) GetDNSName(ctx context.Context, host *host.Host) (string, error) {
	l := m.mutex
	l.RLock()
	instance, ok := m.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return "", errors.Errorf("unable to fetch host '%s'", host.Id)
	}
	return instance.DNSName, nil
}

// terminate an instance
func (m *mockManager) TerminateInstance(ctx context.Context, host *host.Host, user, reason string) error {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := m.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", host.Id)
	}

	instance.Status = StatusTerminated
	m.Instances[host.Id] = instance

	return errors.WithStack(host.Terminate(ctx, user, reason))
}

func (m *mockManager) StopInstance(ctx context.Context, host *host.Host, shouldKeepOff bool, user string) error {
	if !utility.StringSliceContains(evergreen.StoppableHostStatuses, host.Status) {
		return errors.Errorf("cannot stop host '%s' because the host status is '%s' which is not a stoppable state", host.Id, host.Status)
	}

	l := m.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := m.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", host.Id)
	}
	instance.Status = StatusStopped
	m.Instances[host.Id] = instance

	return errors.WithStack(host.SetStopped(ctx, shouldKeepOff, user))

}

func (m *mockManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	if !utility.StringSliceContains(evergreen.StartableHostStatuses, host.Status) {
		return errors.Errorf("cannot start host '%s' because the host status is '%s' which is not a startable state", host.Id, host.Status)
	}

	l := m.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := m.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", host.Id)
	}
	instance.Status = StatusRunning
	m.Instances[host.Id] = instance

	return errors.WithStack(host.SetRunning(ctx, user))
}

func (m *mockManager) RebootInstance(ctx context.Context, host *host.Host, user string) error {
	if host.Status != evergreen.HostRunning {
		return errors.Errorf("cannot reboot host '%s' because the host status is '%s' which is not a rebootable state", host.Id, host.Status)
	}

	l := m.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := m.Instances[host.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", host.Id)
	}
	instance.Status = StatusRunning
	m.Instances[host.Id] = instance

	return errors.WithStack(host.SetRunning(ctx, user))
}

func (m *mockManager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	//no-op. maybe will need to load something from settings in the future.
	return nil
}

func (m *mockManager) TimeTilNextPayment(host *host.Host) time.Duration {
	l := m.mutex
	l.RLock()
	instance, ok := m.Instances[host.Id]
	l.RUnlock()
	if !ok {
		return time.Duration(0)
	}
	return instance.TimeTilNextPayment
}

func (m *mockManager) AttachVolume(ctx context.Context, h *host.Host, attachment *host.VolumeAttachment) error {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	instance, ok := m.Instances[h.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", h.Id)
	}
	instance.BlockDevices = append(instance.BlockDevices, attachment.VolumeID)
	m.Instances[h.Id] = instance

	return errors.WithStack(h.AddVolumeToHost(ctx, attachment))
}

func (m *mockManager) DetachVolume(ctx context.Context, h *host.Host, volumeID string) error {
	l := m.mutex
	l.Lock()
	defer l.Unlock()

	instance, ok := m.Instances[h.Id]
	if !ok {
		return errors.Errorf("unable to fetch host '%s'", h.Id)
	}
	for i := range instance.BlockDevices {
		if volumeID == instance.BlockDevices[i] {
			instance.BlockDevices = append(instance.BlockDevices[:i], instance.BlockDevices[i+1:]...)
		}
	}
	m.Instances[h.Id] = instance

	return errors.WithStack(h.RemoveVolumeFromHost(ctx, volumeID))
}

func (m *mockManager) CreateVolume(ctx context.Context, volume *host.Volume) (*host.Volume, error) {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	if m.Volumes == nil {
		m.Volumes = map[string]MockVolume{}
	}
	if volume.ID == "" {
		volume.ID = primitive.NewObjectID().String()
	}
	m.Volumes[volume.ID] = MockVolume{}
	if err := volume.Insert(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	return volume, nil
}

func (m *mockManager) DeleteVolume(ctx context.Context, volume *host.Volume) error {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	delete(m.Volumes, volume.ID)
	return errors.WithStack(volume.Remove(ctx))
}

func (m *mockManager) ModifyVolume(ctx context.Context, volume *host.Volume, opts *model.VolumeModifyOptions) error {
	l := m.mutex
	l.Lock()
	defer l.Unlock()
	v, ok := m.Volumes[volume.ID]
	if opts.Size > 0 {
		v.Size = opts.Size
		volume.Size = opts.Size
	}
	if !utility.IsZeroTime(opts.Expiration) {
		v.Expiration = opts.Expiration
		volume.Expiration = opts.Expiration
	}
	if opts.NoExpiration {
		v.NoExpiration = true
		volume.NoExpiration = true
	}
	if opts.HasExpiration {
		v.NoExpiration = false
		volume.NoExpiration = false
	}
	if opts.NewName != "" {
		err := volume.SetDisplayName(ctx, opts.NewName)
		if err != nil {
			return err
		}
	}

	if ok {
		m.Volumes[volume.ID] = v
	}

	return nil
}

func (m *mockManager) GetVolumeAttachment(ctx context.Context, volumeID string) (*VolumeAttachment, error) {
	l := m.mutex
	l.Lock()
	defer l.Unlock()

	for id, instance := range m.Instances {
		for _, device := range instance.BlockDevices {
			if device == volumeID {
				return &VolumeAttachment{HostID: id, VolumeID: volumeID}, nil
			}
		}
	}
	return nil, nil
}

// GetInstanceStatus gets the status of all the instances in the slice according
// to the instance data stored in the mock manager.
func (m *mockManager) GetInstanceStatuses(ctx context.Context, hosts []host.Host) (map[string]CloudStatus, error) {
	statuses := map[string]CloudStatus{}
	for _, h := range hosts {
		statuses[h.Id] = m.getOrDefaultInstanceStatus(ctx, h.Id)
	}
	return statuses, nil
}

func (m *mockManager) CheckInstanceType(ctx context.Context, instanceType string) error {
	return nil
}

func (m *mockManager) AssociateIP(ctx context.Context, h *host.Host) error {
	return nil
}

func (m *mockManager) CleanupIP(ctx context.Context, h *host.Host) error {
	return nil
}

// Cleanup is a noop for the mock provider.
func (m *mockManager) Cleanup(context.Context) error {
	return nil
}

// Get mock region from ProviderSettingsList
func getMockManagerOptions(d distro.Distro) (ManagerOpts, error) {
	opts := ManagerOpts{
		Provider: d.Provider,
		Account:  d.ProviderAccount,
	}
	if len(d.ProviderSettingsList) == 0 {
		return opts, nil
	}

	region, ok := d.ProviderSettingsList[0].Lookup("region").StringValueOK()
	if !ok {
		return ManagerOpts{}, errors.New("cannot get region from provider settings")
	}
	opts.Region = region
	return opts, nil
}
