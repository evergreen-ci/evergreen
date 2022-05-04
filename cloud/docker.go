package cloud

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// dockerManager implements the Manager interface for Docker.
type dockerManager struct {
	client DockerClient
	env    evergreen.Environment
}

// ProviderSettings specifies the settings used to configure a host instance.
type dockerSettings struct {
	// ImageURL is the url of the Docker image to use when building the container.
	ImageURL string `mapstructure:"image_url" json:"image_url" bson:"image_url"`
}

// nolint
var (
	// bson fields for the ProviderSettings struct
	imageURLKey = bsonutil.MustHaveTag(dockerSettings{}, "ImageURL")
)

// Validate checks that the settings from the config file are sane.
func (settings *dockerSettings) Validate() error {
	if settings.ImageURL == "" {
		return errors.New("Image must not be empty")
	}

	return nil
}

func (s *dockerSettings) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "error marshalling provider setting into bson")
		}
		if err := bson.Unmarshal(bytes, s); err != nil {
			return errors.Wrap(err, "error unmarshalling bson into provider settings")
		}
	}
	return nil
}

// GetSettings returns an empty ProviderSettings struct.
func (*dockerManager) GetSettings() ProviderSettings {
	return &dockerSettings{}
}

// SpawnHost creates and starts a new Docker container
func (m *dockerManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if !IsDockerProvider(h.Distro.Provider) {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameDocker, h.Distro.Id, h.Distro.Provider)
	}

	if err := h.DockerOptions.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Docker options not valid for host '%s'", h.Id)
	}

	// get parent of host
	parentHost, err := h.GetParent()
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding parent of host '%s'", h.Id)
	}
	hostIP := parentHost.Host
	if hostIP == "" {
		return nil, errors.Wrapf(err, "Error getting host IP for parent host %s", parentHost.Id)
	}

	// Create container
	if err = m.client.CreateContainer(ctx, parentHost, h); err != nil {
		err = errors.Wrapf(err, "Failed to create container for host '%s'", h.Id)
		grip.Info(message.WrapError(err, message.Fields{
			"message": "spawn container host failed",
			"host_id": h.Id,
		}))
		return nil, err
	}

	if err = h.SetAgentRevision(evergreen.AgentVersion); err != nil {
		return nil, errors.Wrapf(err, "error setting agent revision on host %s", h.Id)
	}

	// The setup was successful. Update the container host accordingly in the database.
	if err := h.MarkAsProvisioned(); err != nil {
		return nil, errors.Wrapf(err, "error marking host %s as provisioned", h.Id)
	}

	// Start container
	if err := m.client.StartContainer(ctx, parentHost, h.Id); err != nil {
		err = errors.Wrapf(err, "Docker start container API call failed for host '%s'", hostIP)
		// Clean up
		if err2 := m.client.RemoveContainer(ctx, parentHost, h.Id); err2 != nil {
			err = errors.Wrapf(err, "Unable to cleanup: %+v", err2)
		}
		grip.Info(message.WrapError(err, message.Fields{
			"message": "start container host failed",
			"host_id": h.Id,
		}))
		return nil, err
	}

	grip.Info(message.Fields{
		"message": "created and started Docker container",
		"host_id": h.Id,
	})

	return h, nil
}

func (m *dockerManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances with docker provider")
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
		if client.IsErrConnectionFailed(err) {
			return StatusTerminated, nil
		}
		if client.IsErrNotFound(err) {
			return StatusNonExistent, nil
		}
		return StatusUnknown, errors.Wrapf(err, "Failed to get container information for host '%s'", h.Id)
	}
	return toEvgStatus(container.State), nil
}

func (m *dockerManager) SetPortMappings(ctx context.Context, h, parent *host.Host) error {
	container, err := m.client.GetContainer(ctx, parent, h.Id)
	if err != nil {
		if client.IsErrConnectionFailed(err) {
			return errors.Wrapf(err, "error making connection")
		}
		return errors.Wrapf(err, "Failed to get container information for host '%s'", h.Id)
	}
	if !container.State.Running {
		return errors.Errorf("host '%s' is not running", h.Id)

	}

	if err = h.SetPortMapping(host.GetPortMap(container.NetworkSettings.Ports)); err != nil {
		return errors.Wrapf(err, "error saving ports to host '%s", h.Id)
	}
	return nil
}

// GetDNSName does nothing, returning an empty string and no error.
func (m *dockerManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	return "", nil
}

//TerminateInstance destroys a container.
func (m *dockerManager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	if h.Status == evergreen.HostTerminated {
		err := errors.Errorf("Can not terminate %s - already marked as terminated!", h.Id)
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
	return h.Terminate(user, reason)
}

func (m *dockerManager) StopInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StopInstance is not supported for docker provider")
}

func (m *dockerManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for docker provider")
}

//Configure populates a dockerManager by reading relevant settings from the
//config object.
func (m *dockerManager) Configure(ctx context.Context, s *evergreen.Settings) error {
	config := s.Providers.Docker

	if m.client == nil {
		m.client = GetDockerClient(s)
	}

	if err := m.client.Init(config.APIVersion); err != nil {
		return errors.Wrap(err, "Failed to initialize client connection")
	}

	if m.env == nil {
		return errors.New("docker manager requires access to the evergreen environment")
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

// Cleanup is a noop for the docker provider.
func (m *dockerManager) Cleanup(context.Context) error {
	return nil
}

func (m *dockerManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with docker provider")
}

func (m *dockerManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with docker provider")
}

func (m *dockerManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with docker provider")
}

func (m *dockerManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with docker provider")
}

func (m *dockerManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with docker provider")
}

func (m *dockerManager) GetVolumeAttachment(context.Context, string) (*host.VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with docker provider")
}

func (m *dockerManager) CheckInstanceType(context.Context, string) error {
	return errors.New("can't specify instance type with docker provider")
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
		name := container.Names[0]
		// names in Docker have leading slashes -- https://github.com/moby/moby/issues/6705
		if !strings.HasPrefix(name, "/") {
			return nil, errors.New("error reading container name")
		}
		name = name[1:]
		ids = append(ids, name)
	}

	return ids, nil
}

// canImageBeRemoved returns true if there are no containers running the image
func (m *dockerManager) canImageBeRemoved(ctx context.Context, h *host.Host, imageID string) (bool, error) {
	containers, err := m.client.ListContainers(ctx, h)
	if err != nil {
		return false, errors.Wrap(err, "error listing containers")
	}

	for _, container := range containers {
		if container.ImageID == imageID {
			return false, nil
		}
	}
	return true, nil
}

// RemoveOldestImage finds the oldest image without running containers and forcibly removes it
func (m *dockerManager) RemoveOldestImage(ctx context.Context, h *host.Host) error {
	// list images in order of most to least recently created
	images, err := m.client.ListImages(ctx, h)
	if err != nil {
		return errors.Wrap(err, "Error listing images")
	}

	for i := len(images) - 1; i >= 0; i-- {
		id := images[i].ID
		canBeRemoved, err := m.canImageBeRemoved(ctx, h, id)
		if err != nil {
			return errors.Wrapf(err, "Error checking whether containers are running on image '%s'", id)
		}
		// remove image based on ID only if there are no containers running the image
		if canBeRemoved {
			err = m.client.RemoveImage(ctx, h, id)
			if err != nil {
				return errors.Wrapf(err, "Error removing image '%s'", id)
			}
			return nil
		}
	}
	return nil
}

// CalculateImageSpaceUsage returns the amount of bytes that images take up on disk
func (m *dockerManager) CalculateImageSpaceUsage(ctx context.Context, h *host.Host) (int64, error) {
	images, err := m.client.ListImages(ctx, h)
	if err != nil {
		return 0, errors.Wrap(err, "Error listing images")
	}

	spaceBytes := int64(0)
	for _, image := range images {
		spaceBytes += image.Size
	}
	return spaceBytes, nil
}

// GetContainerImage downloads a container image onto given parent, using given Image. If specified, build image with evergreen agent.
func (m *dockerManager) GetContainerImage(ctx context.Context, parent *host.Host, options host.DockerOptions) error {
	start := time.Now()
	if !parent.HasContainers {
		return errors.Errorf("Error provisioning image: '%s' is not a parent", parent.Id)
	}

	// Import correct base image if not already on host.
	image, err := m.client.EnsureImageDownloaded(ctx, parent, options)
	if err != nil {
		return errors.Wrapf(err, "Unable to ensure that image '%s' is on host '%s'", options.Image, parent.Id)
	}
	grip.Info(message.Fields{
		"operation": "EnsureImageDownloaded",
		"details":   "total",
		"host_id":   parent.Id,
		"image":     image,
		"duration":  time.Since(start),
		"span":      time.Since(start).String(),
	})

	if options.SkipImageBuild {
		return nil
	}

	// Build image containing Evergreen executable.
	_, err = m.client.BuildImageWithAgent(ctx, parent, image)
	if err != nil {
		return errors.Wrapf(err, "Failed to build image '%s' with agent on host '%s'", options.Image, parent.Id)
	}
	grip.Info(message.Fields{
		"operation": "BuildImageWithAgent",
		"host_id":   parent.Id,
		"details":   "total",
		"duration":  time.Since(start),
		"span":      time.Since(start).String(),
	})

	return nil
}

func (m *dockerManager) AddSSHKey(ctx context.Context, pair evergreen.SSHKeyPair) error {
	return nil
}
