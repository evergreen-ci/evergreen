package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ProviderSettings exposes provider-specific configuration settings for a Manager.
type ProviderSettings interface {
	Validate() error

	// If zone is specified, returns the provider settings for that region.
	// This is currently only being implemented for EC2 hosts.
	FromDistroSettings(distro.Distro, string) error
}

//Manager is an interface which handles creating new hosts or modifying
//them via some third-party API.
type Manager interface {
	// Returns a pointer to the manager's configuration settings struct
	GetSettings() ProviderSettings

	// Load credentials or other settings from the config file
	Configure(context.Context, *evergreen.Settings) error

	// SpawnHost attempts to create a new host by requesting one from the
	// provider's API.
	SpawnHost(context.Context, *host.Host) (*host.Host, error)

	// ModifyHost modifies an existing host
	ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error

	// Get the status of an instance
	GetInstanceStatus(context.Context, *host.Host) (CloudStatus, error)

	// TerminateInstance destroys the host in the underlying provider
	TerminateInstance(context.Context, *host.Host, string, string) error

	// StopInstance stops an instance.
	StopInstance(context.Context, *host.Host, string) error

	// StartInstance starts a stopped instance.
	StartInstance(context.Context, *host.Host, string) error

	// IsUp returns true if the underlying provider has not destroyed the
	// host (in other words, if the host "should" be reachable. This does not
	// necessarily mean that the host actually *is* reachable via SSH
	IsUp(context.Context, *host.Host) (bool, error)

	// Called by the hostinit process when the host is actually up. Used
	// to set additional provider-specific metadata
	OnUp(context.Context, *host.Host) error

	// GetDNSName returns the DNS name of a host.
	GetDNSName(context.Context, *host.Host) (string, error)

	// AttachVolume attaches a volume to a host.
	AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error

	// DetachVolume detaches a volume from a host.
	DetachVolume(context.Context, *host.Host, string) error

	// CreateVolume creates a new volume for attaching to a host.
	CreateVolume(context.Context, *host.Volume) (*host.Volume, error)

	// DeleteVolume deletes a volume.
	DeleteVolume(context.Context, *host.Volume) error

	// CheckInstanceType determines if the given instance type is available in the current region.
	CheckInstanceType(context.Context, string) error

	// TimeTilNextPayment returns how long there is until the next payment
	// is due for a particular host
	TimeTilNextPayment(*host.Host) time.Duration

	// AddSSHKey adds an SSH key for this manager's hosts. Adding an existing
	// key is a no-op.
	AddSSHKey(context.Context, evergreen.SSHKeyPair) error
}

type ContainerManager interface {
	Manager

	// GetContainers returns the IDs of all running containers on a specified host
	GetContainers(context.Context, *host.Host) ([]string, error)
	// RemoveOldestImage removes the earliest created image on a specified host
	RemoveOldestImage(ctx context.Context, h *host.Host) error
	// CalculateImageSpaceUsage returns the total space taken up by docker images on a specified host
	CalculateImageSpaceUsage(ctx context.Context, h *host.Host) (int64, error)
	// GetContainerImage downloads a container image onto parent specified by URL, and builds evergreen agent unless otherwise specified
	GetContainerImage(ctx context.Context, parent *host.Host, options host.DockerOptions) error
}

// CostCalculator is an interface for cloud providers that can estimate what a span of time on a
// given host costs.
type CostCalculator interface {
	CostForDuration(context.Context, *host.Host, time.Time, time.Time) (float64, error)
}

// BatchManager is an interface for cloud providers that support batch operations.
type BatchManager interface {
	// GetInstanceStatuses gets the status of a slice of instances.
	GetInstanceStatuses(context.Context, []host.Host) ([]CloudStatus, error)
}

// ManagerOpts is a struct containing the fields needed to get a new cloud manager
// of the proper type.
type ManagerOpts struct {
	Provider       string
	Region         string
	ProviderKey    string
	ProviderSecret string
}

func GetSettings(provider string) (ProviderSettings, error) {
	switch provider {
	case evergreen.ProviderNameEc2OnDemand, evergreen.ProviderNameEc2Spot, evergreen.ProviderNameEc2Auto, evergreen.ProviderNameEc2Fleet:
		return &EC2ProviderSettings{}, nil
	case evergreen.ProviderNameStatic:
		return &StaticSettings{}, nil
	case evergreen.ProviderNameMock:
		return &MockProviderSettings{}, nil
	case evergreen.ProviderNameDocker, evergreen.ProviderNameDockerMock:
		return &dockerSettings{}, nil
	case evergreen.ProviderNameOpenstack:
		return &openStackSettings{}, nil
	case evergreen.ProviderNameGce:
		return &GCESettings{}, nil
	case evergreen.ProviderNameVsphere:
		return &vsphereSettings{}, nil
	}
	return nil, errors.Errorf("invalid provider name %s", provider)
}

// GetManager returns an implementation of Manager for the given manager options.
// It returns an error if the provider name doesn't have a known implementation.
func GetManager(ctx context.Context, env evergreen.Environment, mgrOpts ManagerOpts) (Manager, error) {
	var provider Manager

	switch mgrOpts.Provider {
	case evergreen.ProviderNameEc2OnDemand:
		provider = &ec2Manager{
			env: env,
			EC2ManagerOptions: &EC2ManagerOptions{
				client:         &awsClientImpl{},
				provider:       onDemandProvider,
				region:         mgrOpts.Region,
				providerKey:    mgrOpts.ProviderKey,
				providerSecret: mgrOpts.ProviderSecret,
			},
		}
	case evergreen.ProviderNameEc2Spot:
		provider = &ec2Manager{
			env: env,
			EC2ManagerOptions: &EC2ManagerOptions{
				client:         &awsClientImpl{},
				provider:       spotProvider,
				region:         mgrOpts.Region,
				providerKey:    mgrOpts.ProviderKey,
				providerSecret: mgrOpts.ProviderSecret,
			},
		}
	case evergreen.ProviderNameEc2Auto:
		provider = &ec2Manager{
			env: env,
			EC2ManagerOptions: &EC2ManagerOptions{
				client:         &awsClientImpl{},
				provider:       autoProvider,
				region:         mgrOpts.Region,
				providerKey:    mgrOpts.ProviderKey,
				providerSecret: mgrOpts.ProviderSecret,
			},
		}
	case evergreen.ProviderNameEc2Fleet:
		provider = &ec2FleetManager{
			env: env,
			EC2FleetManagerOptions: &EC2FleetManagerOptions{
				client:         &awsClientImpl{},
				region:         mgrOpts.Region,
				providerKey:    mgrOpts.ProviderKey,
				providerSecret: mgrOpts.ProviderSecret,
			},
		}
	case evergreen.ProviderNameStatic:
		provider = &staticManager{}
	case evergreen.ProviderNameMock:
		provider = makeMockManager()
	case evergreen.ProviderNameDocker:
		provider = &dockerManager{env: env}
	case evergreen.ProviderNameDockerMock:
		provider = &dockerManager{env: env, client: &dockerClientMock{}}
	case evergreen.ProviderNameOpenstack:
		provider = &openStackManager{}
	case evergreen.ProviderNameGce:
		provider = &gceManager{}
	case evergreen.ProviderNameVsphere:
		provider = &vsphereManager{}
	default:
		return nil, errors.Errorf("No known provider for '%s'", mgrOpts.Provider)
	}

	if err := provider.Configure(ctx, env.Settings()); err != nil {
		return nil, errors.Wrap(err, "Failed to configure cloud provider")
	}

	return provider, nil
}

// GetManagerOptions gets the manager options from the provider settings object for a given
// provider name.
func GetManagerOptions(d distro.Distro) (ManagerOpts, error) {
	if len(d.ProviderSettingsList) > 1 {
		return ManagerOpts{}, errors.Errorf("distro should be modified to have only one provider settings")
	}
	if IsEc2Provider(d.Provider) {
		s := &EC2ProviderSettings{}
		if err := s.FromDistroSettings(d, ""); err != nil {
			return ManagerOpts{}, errors.Wrapf(err, "error getting EC2 provider settings from distro")
		}

		return getEC2ManagerOptionsFromSettings(d.Provider, s), nil
	}
	if d.Provider == evergreen.ProviderNameMock {
		return getMockManagerOptions(d.Provider, d.ProviderSettings)
	}
	return ManagerOpts{Provider: d.Provider}, nil
}

// ConvertContainerManager converts a regular manager into a container manager,
// errors if type conversion not possible.
func ConvertContainerManager(m Manager) (ContainerManager, error) {
	if cm, ok := m.(ContainerManager); ok {
		return cm, nil
	}
	return nil, errors.New("Error converting manager to container manager")
}

// If ProviderSettings is populated, then it was modified via UI and we should save this to the list.
// Otherwise, repopulate ProviderSettings from the list to maintain the UI. This is only necessary temporarily.
func UpdateProviderSettings(d *distro.Distro) error {
	if d.ProviderSettings != nil && len(*d.ProviderSettings) > 0 {
		if err := CreateSettingsListFromLegacy(d); err != nil {
			return errors.Wrapf(err, "error creating new settings list for distro '%s'", d.Id)
		}
	} else if len(d.ProviderSettingsList) > 0 {
		region := ""
		if len(d.ProviderSettingsList) > 1 {
			region = evergreen.DefaultEC2Region
		}
		doc, err := d.GetProviderSettingByRegion(region)
		if err != nil {
			return errors.Wrapf(err, "unable to get default provider settings for distro '%s'", d.Id)
		}
		docMap := doc.ExportMap()
		d.ProviderSettings = &docMap
	}
	return nil
}

func CreateSettingsListFromLegacy(d *distro.Distro) error {
	bytes, err := bson.Marshal(d.ProviderSettings)
	if err != nil {
		return errors.Wrap(err, "error marshalling provider setting into bson")
	}
	doc := &birch.Document{}
	if err := doc.UnmarshalBSON(bytes); err != nil {
		return errors.Wrapf(err, "error unmarshalling settings bytes into document")
	}
	if IsEc2Provider(d.Provider) {
		doc = doc.Set(birch.EC.String("region", evergreen.DefaultEC2Region))
	}
	d.ProviderSettingsList = []*birch.Document{doc}
	return nil
}
