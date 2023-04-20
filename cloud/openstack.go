package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gophercloud/gophercloud"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// openStackManager implements the Manager interface for OpenStack.
type openStackManager struct {
	authOptions  *gophercloud.AuthOptions
	endpointOpts *gophercloud.EndpointOpts
	client       openStackClient
}

// ProviderSettings specifies the settings used to configure a host instance.
type openStackSettings struct {
	ImageName     string `mapstructure:"image_name" json:"image_name" bson:"image_name"`
	FlavorName    string `mapstructure:"flavor_name" json:"flavor_name" bson:"flavor_name"`
	KeyName       string `mapstructure:"key_name" json:"key_name" bson:"key_name"`
	SecurityGroup string `mapstructure:"security_group" json:"security_group" bson:"security_group"`
}

// Validate verifies a set of ProviderSettings.
func (opts *openStackSettings) Validate() error {
	if opts.ImageName == "" {
		return errors.New("image name must not be blank")
	}

	if opts.FlavorName == "" {
		return errors.New("flavor name must not be blank")
	}

	if opts.KeyName == "" {
		return errors.New("key name must not be blank")
	}

	if opts.SecurityGroup == "" {
		return errors.New("security group must not be blank")
	}

	return nil
}

func (opts *openStackSettings) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "marshalling provider setting into BSON")
		}
		if err := bson.Unmarshal(bytes, opts); err != nil {
			return errors.Wrap(err, "unmarshalling BSON into provider settings")
		}
	}
	return nil
}

// Configure loads the necessary credentials from the global config object.
func (m *openStackManager) Configure(ctx context.Context, s *evergreen.Settings) error {
	config := s.Providers.OpenStack

	m.authOptions = &gophercloud.AuthOptions{
		IdentityEndpoint: config.IdentityEndpoint,
		Username:         config.Username,
		Password:         config.Password,
		DomainName:       config.DomainName,
		TenantName:       config.ProjectName,
		TenantID:         config.ProjectID,
	}

	m.endpointOpts = &gophercloud.EndpointOpts{
		Region: config.Region,
	}

	if m.client == nil {
		m.client = &openStackClientImpl{}
	}

	if err := m.client.Init(*m.authOptions, *m.endpointOpts); err != nil {
		return errors.Wrap(err, "initializing client connection")
	}

	return nil
}

// SpawnHost attempts to create a new host by requesting one from the OpenStack API.
// Information about the intended (and eventually created) host is recorded in a DB document.
//
// ProviderSettings in the distro should have the following settings:
//   - ImageName:     image name
//   - FlavorName:    like an AWS instance type i.e. m1.large
//   - KeyName:       (optional) keypair name associated with the account
//   - SecurityGroup: (optional) security group name
func (m *openStackManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameOpenstack {
		return nil, errors.Errorf("can't spawn instance for distro '%s': distro provider is '%s'", h.Distro.Id, h.Distro.Provider)
	}

	settings := &openStackSettings{}
	if err := settings.FromDistroSettings(h.Distro, ""); err != nil {
		return nil, errors.Wrapf(err, "getting provider settings from distro '%s'", h.Distro.Id)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid provider settings in distro '%s'", h.Distro.Id)
	}

	// Start the instance, and remove the intent host document if unsuccessful.
	opts := getSpawnOptions(h, settings)
	server, err := m.client.CreateInstance(opts, settings.KeyName)
	if err != nil {
		grip.Error(err)
		if rmErr := h.Remove(); rmErr != nil {
			grip.Errorf("Could not remove intent host: %s", message.Fields{
				"host_id": h.Id,
				"error":   rmErr,
			})
		}
		return nil, errors.Wrapf(err, "starting new instances for distro '%s'", h.Distro.Id)
	}

	// Update the ID of the host with the real one
	h.Id = server.ID

	grip.Debug(message.Fields{
		"message":  "new OpenStack host",
		"instance": h.Id,
		"host":     h,
	})

	return h, nil
}

func (m *openStackManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances with OpenStack provider")
}

// GetInstanceStatus gets the current operational status of the provisioned host,
func (m *openStackManager) GetInstanceStatus(ctx context.Context, host *host.Host) (CloudStatus, error) {
	server, err := m.client.GetInstance(host.Id)
	if err != nil {
		return StatusUnknown, err
	}

	return osStatusToEvgStatus(server.Status), nil
}

func (m *openStackManager) SetPortMappings(context.Context, *host.Host, *host.Host) error {
	return errors.New("can't set port mappings with OpenStack provider")
}

// TerminateInstance requests a server previously provisioned to be removed.
func (m *openStackManager) TerminateInstance(ctx context.Context, host *host.Host, user, reason string) error {
	if host.Status == evergreen.HostTerminated {
		return errors.Errorf("cannot terminate host '%s' because it's already marked as terminated", host.Id)
	}

	if err := m.client.DeleteInstance(host.Id); err != nil {
		return errors.Wrap(err, "API call to delete instance failed")
	}

	// Set the host status as terminated and update its termination time
	return errors.WithStack(host.Terminate(user, reason))
}

func (m *openStackManager) StopInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StopInstance is not supported for OpenStack provider")
}

func (m *openStackManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for OpenStack provider")
}

// IsUp checks whether the provisioned host is running.
func (m *openStackManager) IsUp(ctx context.Context, host *host.Host) (bool, error) {
	status, err := m.GetInstanceStatus(ctx, host)
	if err != nil {
		return false, err
	}

	return status == StatusRunning, nil
}

// OnUp does nothing since tags are attached in SpawnInstance.
func (m *openStackManager) OnUp(ctx context.Context, host *host.Host) error {
	return nil
}

// GetDNSName returns the private IP address of the host.
func (m *openStackManager) GetDNSName(ctx context.Context, host *host.Host) (string, error) {
	server, err := m.client.GetInstance(host.Id)
	if err != nil {
		return "", err
	}

	for _, subnet := range server.Addresses {
		addresses, ok := subnet.([]interface{})
		if !ok {
			return "", errors.Errorf("cannot convert type of subnet %+v to []interface{} for host '%s'", subnet, host.Id)
		}

		for _, address := range addresses {
			keyvalues, ok := address.(map[string]interface{})
			if !ok {
				return "", errors.Errorf("cannot convert type of addresses %+v to map[string]interface{} for host '%s'", address, host.Id)
			}

			if ip := keyvalues["addr"]; ip != nil {
				ip, ok = ip.(string)
				if !ok {
					return "", errors.Errorf("cannot convert type of address %+v to string for host '%s'", ip, host.Id)
				}
				return ip.(string), nil
			}
		}
	}

	return "", errors.Errorf("could not find IP for host '%s'", host.Id)
}

func (m *openStackManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with OpenStack provider")
}

func (m *openStackManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with OpenStack provider")
}

func (m *openStackManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volumes with OpenStack provider")
}

func (m *openStackManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volumes with OpenStack provider")
}

func (m *openStackManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with OpenStack provider")
}

func (m *openStackManager) GetVolumeAttachment(context.Context, string) (*VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with OpenStack provider")
}

func (m *openStackManager) CheckInstanceType(context.Context, string) error {
	return errors.New("can't specify instance type with OpenStack provider")
}

// Cleanup is a noop for the openstack provider.
func (m *openStackManager) Cleanup(context.Context) error {
	return nil
}

// TimeTilNextPayment always returns 0. The OpenStack dashboard requires third-party
// plugins for billing, monitoring, and other management tools.
func (m *openStackManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}

func (m *openStackManager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	return nil
}
