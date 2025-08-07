package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type staticManager struct{}

type StaticSettings struct {
	Hosts []StaticHost `mapstructure:"hosts" json:"hosts" bson:"hosts"`
}

type StaticHost struct {
	Name    string `bson:"name" json:"name" mapstructure:"name"`
	SSHPort int    `bson:"ssh_port,omitempty" json:"ssh_port,omitempty" mapstructure:"ssh_port,omitempty"`
}

var (
	// bson fields for the StaticSettings struct
	HostsKey = bsonutil.MustHaveTag(StaticSettings{}, "Hosts")

	// bson fields for the Host struct
	NameKey = bsonutil.MustHaveTag(StaticHost{}, "Name")
)

// Validate checks that the settings from the configuration are valid.
func (s *StaticSettings) Validate() error {
	for _, h := range s.Hosts {
		if h.Name == "" {
			return errors.New("host 'name' field can not be blank")
		}
	}
	return nil
}

func (s *StaticSettings) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "marshalling provider setting into BSON")
		}
		if err := bson.Unmarshal(bytes, s); err != nil {
			return errors.Wrap(err, "unmarshalling BSON into provider settings")
		}
	}
	return nil
}

func (staticMgr *staticManager) SpawnHost(context.Context, *host.Host) (*host.Host, error) {
	return nil, errors.New("cannot start new instances with static provider")
}

func (staticMgr *staticManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("cannot modify instances with static provider")
}

// get the information of an instance
func (staticMgr *staticManager) GetInstanceState(ctx context.Context, host *host.Host) (CloudInstanceState, error) {
	return CloudInstanceState{Status: StatusRunning}, nil
}

// get instance DNS
func (staticMgr *staticManager) GetDNSName(ctx context.Context, host *host.Host) (string, error) {
	return host.Id, nil
}

func (m *staticManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with static provider")
}

// terminate an instance
func (staticMgr *staticManager) TerminateInstance(ctx context.Context, host *host.Host, user, reason string) error {
	// a decommissioned static host will be removed from the database
	if host.Status == evergreen.HostDecommissioned {
		event.LogHostStatusChanged(ctx, host.Id, host.Status, evergreen.HostDecommissioned, evergreen.User, reason)
		grip.Debug(message.Fields{
			"message":  "removing decommissioned static host",
			"distro":   host.Distro.Id,
			"host_id":  host.Id,
			"hostname": host.Host,
		})
		if err := host.Remove(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not remove decommissioned static host",
				"host_id": host.Id,
				"distro":  host.Distro.Id,
			}))
		}
	}

	grip.Debug(message.Fields{
		"message": "cannot terminate a static host",
		"host_id": host.Id,
		"distro":  host.Distro.Id,
	})

	return nil
}

func (staticMgr *staticManager) StopInstance(ctx context.Context, host *host.Host, shouldKeepOff bool, user string) error {
	return errors.New("StopInstance is not supported for static provider")
}

func (staticMgr *staticManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for static provider")
}

func (staticMgr *staticManager) Configure(ctx context.Context, settings *evergreen.Settings) error {
	//no-op. maybe will need to load something from settings in the future.
	return nil
}

func (staticMgr *staticManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with static provider")
}

func (staticMgr *staticManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with static provider")
}

func (staticMgr *staticManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with static provider")
}

func (staticMgr *staticManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with static provider")
}

func (m *staticManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with static provider")
}

func (m *staticManager) GetVolumeAttachment(context.Context, string) (*VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with static provider")
}

func (staticMgr *staticManager) CheckInstanceType(context.Context, string) error {
	return errors.New("can't specify instance type with static provider")
}

func (staticMgr *staticManager) AssociateIP(context.Context, *host.Host) error {
	return errors.New("can't associate IP with static provider")
}

func (staticMgr *staticManager) CleanupIP(context.Context, *host.Host) error {
	return nil
}

// Cleanup is a noop for the static provider.
func (staticMgr *staticManager) Cleanup(context.Context) error {
	return nil
}

// determine how long until a payment is due for the host. static hosts always
// return 0 for this number
func (staticMgr *staticManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
