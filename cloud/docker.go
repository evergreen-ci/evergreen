package cloud

import (
	"context"
	"strings"
	"time"

	"github.com/mongodb/anser/bsonutil"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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

	if h.DockerOptions.Image == "" {
		return nil, errors.Errorf("Docker image empty for host '%s'", h.Id)
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

	settings := dockerSettings{ImageURL: h.DockerOptions.Image}
	if err = settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid Docker settings for host '%s'", h.Id)
	}

	// Create container
	if err = m.client.CreateContainer(ctx, parentHost, h); err != nil {
		err = errors.Wrapf(err, "Failed to create container for host '%s'", h.Id)
		grip.Info(message.WrapError(err, message.Fields{
			"message": "spawn container host failed",
			"host":    h.Id,
		}))
		return nil, err
	}

	if err = h.SetAgentRevision(evergreen.BuildRevision); err != nil {
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
			"host":    h.Id,
		}))
		return nil, err
	}

	grip.Info(message.Fields{
		"message": "created and started Docker container",
		"host":    h.Id,
	})
	event.LogHostStarted(h.Id)

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
		return StatusUnknown, errors.Wrapf(err, "Failed to get container information for host '%v'", h.Id)
	}

	return toEvgStatus(container.State), nil
}

// GetDNSName does nothing, returning an empty string and no error.
func (m *dockerManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	return "", nil
}

//TerminateInstance destroys a container.
func (m *dockerManager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
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

// CostForDuration estimates the cost for a span of time on the given container
// host. The method divides the cost of that span on the parent host by an
// estimate of the number of containers running during the same interval.
func (m *dockerManager) CostForDuration(ctx context.Context, h *host.Host, start, end time.Time) (float64, error) {
	parent, err := h.GetParent()
	if err != nil {
		return 0, errors.Wrapf(err, "Error retrieving parent for host '%s'", h.Id)
	}

	numContainers, err := parent.EstimateNumContainersForDuration(start, end)
	if err != nil {
		return 0, errors.Wrap(err, "Errors estimating number of containers running over interval")
	}

	// prevent division by zero error
	if numContainers == 0 {
		return 0, nil
	}

	// get cloud manager for parent
	mgrOpts := ManagerOpts{
		Provider: parent.Provider,
		Region:   GetRegion(parent.Distro),
	}
	parentMgr, err := GetManager(ctx, m.env, mgrOpts)
	if err != nil {
		return 0, errors.Wrapf(err, "Error loading provider for parent host '%s'", parent.Id)
	}

	// get parent cost for time interval
	calc, ok := parentMgr.(CostCalculator)
	if !ok {
		return 0, errors.Errorf("Type assertion failed: type %T does not hold a CostCaluclator", parentMgr)
	}
	cost, err := calc.CostForDuration(ctx, parent, start, end)
	if err != nil {
		return 0, errors.Wrapf(err, "Error calculating cost for parent host '%s'", parent.Id)
	}

	return cost / numContainers, nil
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
		"details":   "total",
		"duration":  time.Since(start),
		"span":      time.Since(start).String(),
	})

	return nil
}
