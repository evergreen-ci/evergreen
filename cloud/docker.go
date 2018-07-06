package cloud

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// dockerManager implements the Manager interface for Docker.
type dockerManager struct {
	client dockerClient
}

// ProviderSettings specifies the settings used to configure a host instance.
type dockerSettings struct {
	// ImageID is the Docker image ID already loaded on the host machine.
	ImageID string `mapstructure:"image_name" json:"image_name" bson:"image_name"`
}

// nolint
var (
	// bson fields for the ProviderSettings struct
	imageIDKey = bsonutil.MustHaveTag(dockerSettings{}, "ImageID")
)

//Validate checks that the settings from the config file are sane.
func (settings *dockerSettings) Validate() error {
	if settings.ImageID == "" {
		return errors.New("ImageName must not be blank")
	}

	return nil
}

// GetSettings returns an empty ProviderSettings struct.
func (*dockerManager) GetSettings() ProviderSettings {
	return &dockerSettings{}
}

// SpawnHost creates and starts a new Docker container
func (m *dockerManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameDocker {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameDocker, h.Distro.Id, h.Distro.Provider)
	}

	// Decode provider settings from distro settings
	settings := &dockerSettings{}
	if h.Distro.ProviderSettings != nil {
		if err := mapstructure.Decode(h.Distro.ProviderSettings, settings); err != nil {
			return nil, errors.Wrapf(err, "Error decoding params for distro '%s'", h.Distro.Id)
		}
	}

	// get parent of host
	parent, err := h.GetParent()
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding parent of host '%s'", h.Id)
	}
	hostIP := parent.Host
	if hostIP == "" {
		return nil, errors.Wrapf(err, "Error getting host IP for parent host %s", parent.Id)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid Docker settings for host '%s'", h.Id)
	}

	grip.Info(message.Fields{
		"message":   "decoded Docker container settings",
		"container": h.Id,
		"host_ip":   hostIP,
		"image_id":  settings.ImageID,
	})

	// Create container
	if err := m.client.CreateContainer(ctx, parent, h.Id, settings); err != nil {
		err = errors.Wrapf(err, "Failed to create container for host '%s'", hostIP)
		grip.Error(err)
		return nil, err
	}

	// Start container
	if err := m.client.StartContainer(ctx, parent, h.Id); err != nil {
		err = errors.Wrapf(err, "Docker start container API call failed for host '%s'", hostIP)
		// Clean up
		if err2 := m.client.RemoveContainer(ctx, parent, h.Id); err2 != nil {
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
	newContainer, err := m.client.GetContainer(ctx, parent, h.Id)
	if err != nil {
		err = errors.Wrapf(err, "Docker inspect container API call failed for host '%s'", hostIP)
		grip.Error(err)
		return nil, err
	}

	hostPort, err := retrieveOpenPortBinding(newContainer)
	if err != nil {
		err = errors.Wrapf(err, "Container '%s' could not retrieve open ports", newContainer.ID)
		grip.Error(err)
		return nil, err
	}
	h.Host = fmt.Sprintf("%s:%s", hostIP, hostPort)

	grip.Info(message.Fields{
		"message":   "retrieved open port binding",
		"container": h.Id,
		"host_ip":   hostIP,
		"host_port": hostPort,
	})

	return h, nil
}

// GetInstanceStatus returns a universal status code representing the state
// of a container.
func (m *dockerManager) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {
	// get parent of container host
	parent, err := h.GetParent()
	if err != nil {
		return StatusUnknown, errors.Wrapf(err, "Error retrieving parent of host '%s'", h.Id)
	}

	container, err := m.client.GetContainer(ctx, parent, h.Id)
	if err != nil {
		return StatusUnknown, errors.Wrapf(err, "Failed to get container information for host '%v'", h.Id)
	}

	return toEvgStatus(container.State), nil
}

//GetDNSName gets the DNS hostname of a container by reading it directly from
//the Docker API
func (m *dockerManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	if h.Host == "" {
		return "", errors.New("DNS name is empty")
	}
	return h.Host, nil
}

//TerminateInstance destroys a container.
func (m *dockerManager) TerminateInstance(ctx context.Context, h *host.Host, user string) error {
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", h.Id)
		grip.Error(err)
		return err
	}

	// get parent of container host
	parent, err := h.GetParent()
	if err != nil {
		return errors.Wrapf(err, "Error retrieving parent for host '%s'", h.Id)
	}

	if err := m.client.RemoveContainer(ctx, parent, h.Id); err != nil {
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
func (m *dockerManager) Configure(ctx context.Context, s *evergreen.Settings) error {
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
func (m *dockerManager) IsUp(ctx context.Context, h *host.Host) (bool, error) {
	cloudStatus, err := m.GetInstanceStatus(ctx, h)
	if err != nil {
		return false, err
	}
	return cloudStatus == StatusRunning, nil
}

// OnUp does nothing.
func (m *dockerManager) OnUp(context.Context, *host.Host) error {
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

func (m *dockerManager) GetContainers(ctx context.Context, h *host.Host) ([]string, error) {
	containers, err := m.client.ListContainers(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "error listing containers")
	}

	ids := []string{}
	for _, container := range containers {
		ids = append(ids, container.ID)
	}

	return ids, nil
}
