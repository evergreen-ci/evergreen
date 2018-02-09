// +build go1.7

package cloud

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// dockerManager implements the CloudManager interface for Docker.
type dockerManager struct {
	client dockerClient
}

type portRange struct {
	MinPort uint16 `mapstructure:"min_port" json:"min_port" bson:"min_port"`
	MaxPort uint16 `mapstructure:"max_port" json:"max_port" bson:"max_port"`
}

// ProviderSettings specifies the settings used to configure a host instance.
type dockerSettings struct {
	// HostIP is the IP address of the machine on which to start Docker containers. This
	// host machine must already have Docker installed and the Docker API exposed at the
	// client port, and preloaded Docker images.
	HostIP string `mapstructure:"host_ip" json:"host_ip" bson:"host_ip"`
	// ImageID is the Docker image ID already loaded on the host machine.
	ImageID string `mapstructure:"image_name" json:"image_name" bson:"image_name"`
	// ClientPort is the port at which the Docker API is exposed on the host machine.
	ClientPort int `mapstructure:"client_port" json:"client_port" bson:"client_port"`
	// PortRange specifies potential ports to bind new containers to for SSH connections.
	PortRange *portRange `mapstructure:"port_range" json:"port_range" bson:"port_range"`
}

// nolint
var (
	// bson fields for the ProviderSettings struct
	hostIPKey     = bsonutil.MustHaveTag(dockerSettings{}, "HostIP")
	imageIDKey    = bsonutil.MustHaveTag(dockerSettings{}, "ImageID")
	clientPortKey = bsonutil.MustHaveTag(dockerSettings{}, "ClientPort")
	portRangeKey  = bsonutil.MustHaveTag(dockerSettings{}, "PortRange")

	// bson fields for the portRange struct
	minPortKey = bsonutil.MustHaveTag(portRange{}, "MinPort")
	maxPortKey = bsonutil.MustHaveTag(portRange{}, "MaxPort")
)

//Validate checks that the settings from the config file are sane.
func (settings *dockerSettings) Validate() error {
	if settings.HostIP == "" {
		return errors.New("HostIP must not be blank")
	}

	if settings.ImageID == "" {
		return errors.New("ImageName must not be blank")
	}

	if settings.ClientPort == 0 {
		return errors.New("Port must not be blank")
	}

	if settings.PortRange != nil {
		min := settings.PortRange.MinPort
		max := settings.PortRange.MaxPort

		if max < min {
			return errors.New("Container port range must be valid")
		}
	}

	return nil
}

// GetSettings returns an empty ProviderSettings struct.
func (*dockerManager) GetSettings() ProviderSettings {
	return &dockerSettings{}
}

//GetInstanceName returns a name to be used for an instance
func (*dockerManager) GetInstanceName(_ *distro.Distro) string {
	return fmt.Sprintf("container-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

// SpawnHost creates and starts a new Docker container
func (m *dockerManager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameDocker {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameDocker, h.Distro.Id, h.Distro.Provider)
	}

	// Decode provider settings from distro settings
	settings := &dockerSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro '%s'", h.Distro.Id)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid Docker settings in distro '%s'", h.Distro.Id)
	}

	grip.Info(message.Fields{
		"message":     "decoded Docker container settings",
		"container":   h.Id,
		"host_ip":     settings.HostIP,
		"image_id":    settings.ImageID,
		"client_port": settings.ClientPort,
		"min_port":    settings.PortRange.MinPort,
		"max_port":    settings.PortRange.MaxPort,
	})

	// Create container
	if err := m.client.CreateContainer(h.Id, &h.Distro, settings); err != nil {
		err = errors.Wrapf(err, "Failed to create container for host '%s'", settings.HostIP)
		grip.Error(err)
		return nil, err
	}

	// Start container
	if err := m.client.StartContainer(h); err != nil {
		err = errors.Wrapf(err, "Docker start container API call failed for host '%s'", settings.HostIP)
		// Clean up
		if err2 := m.client.RemoveContainer(h); err2 != nil {
			err = errors.Wrapf(err, "Unable to cleanup: %+v", err2)
		}
		grip.Error(err)
		return nil, err
	}

	grip.Info(message.Fields{
		"message":   "created and started Docker container",
		"container": h.Id,
	})
	event.LogHostStarted(h.Id)

	// Retrieve container details
	newContainer, err := m.client.GetContainer(h)
	if err != nil {
		err = errors.Wrapf(err, "Docker inspect container API call failed for host '%s'", settings.HostIP)
		grip.Error(err)
		return nil, err
	}

	hostPort, err := retrieveOpenPortBinding(newContainer)
	if err != nil {
		err = errors.Wrapf(err, "Container '%s' could not retrieve open ports", newContainer.ID)
		grip.Error(err)
		return nil, err
	}
	h.Host = fmt.Sprintf("%s:%s", settings.HostIP, hostPort)

	grip.Info(message.Fields{
		"message":   "retrieved open port binding",
		"container": h.Id,
		"host_ip":   settings.HostIP,
		"host_port": hostPort,
	})

	return h, nil
}

// GetInstanceStatus returns a universal status code representing the state
// of a container.
func (m *dockerManager) GetInstanceStatus(h *host.Host) (CloudStatus, error) {
	container, err := m.client.GetContainer(h)
	if err != nil {
		return StatusUnknown, errors.Wrapf(err, "Failed to get container information for host '%v'", h.Id)
	}

	return toEvgStatus(container.State), nil
}

//GetDNSName gets the DNS hostname of a container by reading it directly from
//the Docker API
func (m *dockerManager) GetDNSName(h *host.Host) (string, error) {
	if h.Host == "" {
		return "", errors.New("DNS name is empty")
	}
	return h.Host, nil
}

//CanSpawn returns if a given cloud provider supports spawning a new host
//dynamically. Always returns true for Docker.
func (m *dockerManager) CanSpawn() (bool, error) {
	return true, nil
}

//TerminateInstance destroys a container.
func (m *dockerManager) TerminateInstance(h *host.Host, user string) error {
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", h.Id)
		grip.Error(err)
		return err
	}

	if err := m.client.RemoveContainer(h); err != nil {
		return errors.Wrap(err, "API call to remove container failed")
	}

	grip.Info(message.Fields{
		"message":   "terminated Docker container",
		"container": h.Id,
	})

	// Set the host status as terminated and update its termination time
	return h.Terminate(user)
}

//Configure populates a dockerManager by reading relevant settings from the
//config object.
func (m *dockerManager) Configure(s *evergreen.Settings) error {
	config := s.Providers.Docker

	if m.client == nil {
		m.client = &dockerClientImpl{}
	}

	if err := m.client.Init(config.APIVersion); err != nil {
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	return nil
}

//IsUp checks the container's state by querying the Docker API and
//returns true if the host should be available to connect with SSH.
func (m *dockerManager) IsUp(h *host.Host) (bool, error) {
	cloudStatus, err := m.GetInstanceStatus(h)
	if err != nil {
		return false, err
	}
	return cloudStatus == StatusRunning, nil
}

// OnUp does nothing.
func (m *dockerManager) OnUp(_ *host.Host) error {
	return nil
}

//GetSSHOptions returns an array of default SSH options for connecting to a
//container.
func (m *dockerManager) GetSSHOptions(h *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, errors.New("No key specified for Docker host")
	}

	opts := []string{"-i", keyPath}
	for _, opt := range h.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host. For Docker this is not relevant.
func (m *dockerManager) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}
