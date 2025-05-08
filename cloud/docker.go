package cloud

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// dockerManager implements the Manager interface for Docker.
type dockerManager struct {
	client DockerClient
	env    evergreen.Environment
}

// dockerSettings are an empty placeholder to fulfill the ProviderSettings
// interface.
type dockerSettings struct{}

// Validate is a no-op.
func (*dockerSettings) Validate() error { return nil }

// FromDistroSettings is a no-op.
func (*dockerSettings) FromDistroSettings(distro.Distro, string) error { return nil }

// SpawnHost creates and starts a new Docker container
func (m *dockerManager) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {
	if !evergreen.IsDockerProvider(h.Distro.Provider) {
		return nil, errors.Errorf("can't spawn instance of provider '%s' for distro '%s': distro provider is '%s'", evergreen.ProviderNameDocker, h.Distro.Id, h.Distro.Provider)
	}

	if err := h.DockerOptions.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Docker options not valid for host '%s'", h.Id)
	}

	// get parent of host
	parentHost, err := h.GetParent(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "finding parent of host '%s'", h.Id)
	}
	hostIP := parentHost.Host
	if hostIP == "" {
		return nil, errors.Wrapf(err, "getting host IP for parent host '%s'", parentHost.Id)
	}

	// Create container
	if err = m.client.CreateContainer(ctx, parentHost, h); err != nil {
		err = errors.Wrapf(err, "creating container for host '%s'", h.Id)
		grip.Info(message.WrapError(err, message.Fields{
			"message": "spawn container host failed",
			"host_id": h.Id,
		}))
		return nil, err
	}

	if err := m.attachStdinStream(parentHost, h); err != nil {
		return nil, errors.Wrapf(err, "attaching stdin to container for host '%s'", h.Id)
	}

	if err = h.SetAgentRevision(ctx, evergreen.AgentVersion); err != nil {
		return nil, errors.Wrapf(err, "setting agent revision on host '%s' to '%s'", h.Id, evergreen.AgentVersion)
	}

	// The setup was successful. Update the container host accordingly in the database.
	if err := h.MarkAsProvisioned(ctx); err != nil {
		return nil, errors.Wrapf(err, "marking host '%s' as provisioned", h.Id)
	}

	// Start container
	if err := m.client.StartContainer(ctx, parentHost, h.Id); err != nil {
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(err, "starting container for host '%s'", hostIP)
		// Clean up
		if err := m.client.RemoveContainer(ctx, parentHost, h.Id); err != nil {
			catcher.Wrap(err, "removing container due to failure to start container")
		}
		grip.Info(message.WrapError(catcher.Resolve(), message.Fields{
			"message": "start container host failed",
			"host_id": h.Id,
		}))
		return nil, catcher.Resolve()
	}

	grip.Info(message.Fields{
		"message": "created and started Docker container",
		"host_id": h.Id,
	})

	return h, nil
}

// attachStdinStream attaches to the container's stdin and streams data to it.
// Note that the stream to stdin has to run in the background because stdin is
// not open until the container command is up and running.
func (m *dockerManager) attachStdinStream(parent *host.Host, container *host.Host) error {
	if len(container.DockerOptions.StdinData) == 0 {
		return nil
	}

	// This needs to use a context that's not attached to the container creation
	// request because the request does not live as long as the container does.
	// For example, the context could be from a REST request to start a
	// container, and the container will still be starting/running after the
	// REST request finishes. Streaming to stdin occurs asynchronously once the
	// container is up and running, so we have to ensure that the attached stdin
	// lives beyond the request and eventually streams to the running container.
	var timeoutAt time.Time
	if !utility.IsZeroTime(container.ExpirationTime) {
		timeoutAt = container.ExpirationTime
	}
	if !utility.IsZeroTime(container.SpawnOptions.TimeoutTeardown) {
		timeoutAt = container.SpawnOptions.TimeoutTeardown
	}
	if utility.IsZeroTime(timeoutAt) {
		// There's no reasonable deadline to use, so just time out after a
		// little bit. Realistically, the data will likely stream to the
		// container well within this deadline.
		timeoutAt = time.Now().Add(15 * time.Minute)
	}
	stdinCtx, stdinCancel := context.WithDeadline(context.Background(), timeoutAt)

	dockerOpts := container.DockerOptions

	// Once the stdin data is used, clear it from the host to be safe in case it
	// contains sensitive data.
	grip.Error(message.WrapError(container.ClearDockerStdinData(stdinCtx), message.Fields{
		"message":        "could not clear Docker stdin data from container, so it may linger in the document",
		"container":      container.Id,
		"task_id":        container.SpawnOptions.TaskID,
		"task_execution": container.SpawnOptions.TaskExecutionNumber,
		"build_id":       container.SpawnOptions.BuildID,
	}))

	stream, err := m.client.AttachToContainer(stdinCtx, parent, container.Id, dockerOpts)
	if err != nil {
		stdinCancel()
		return errors.Wrap(err, "attaching stdin stream to container")
	}

	go func() {
		defer stdinCancel()
		m.runContainerStdinStream(stream, container, dockerOpts.StdinData)
	}()

	return nil
}

// runContainerStdinStream streams data to the container's stdin.
func (m *dockerManager) runContainerStdinStream(stream *types.HijackedResponse, container *host.Host, stdinData []byte) {
	defer func() {
		if err := recovery.HandlePanicWithError(recover(), nil, "streaming stdin to container"); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":        "panicked while streaming stdin to container",
				"container":      container.Id,
				"task_id":        container.SpawnOptions.TaskID,
				"task_execution": container.SpawnOptions.TaskExecutionNumber,
				"build_id":       container.SpawnOptions.BuildID,
			}))
		}

		stream.Close()
	}()

	_, err := io.Copy(stream.Conn, bytes.NewBuffer(stdinData))
	grip.Error(message.WrapError(err, message.Fields{
		"message":        "could not stream stdin data to container",
		"container":      container.Id,
		"task_id":        container.SpawnOptions.TaskID,
		"task_execution": container.SpawnOptions.TaskExecutionNumber,
		"build_id":       container.SpawnOptions.BuildID,
	}))
}

func (m *dockerManager) ModifyHost(context.Context, *host.Host, host.HostModifyOptions) error {
	return errors.New("can't modify instances with docker provider")
}

// GetInstanceState returns a universal status code representing the state
// of a container and a state reason if available. The state reason should not be
// used to determine the status of the container but rather to provide additional
// context about the state of the container.
func (m *dockerManager) GetInstanceState(ctx context.Context, h *host.Host) (CloudInstanceState, error) {
	info := CloudInstanceState{Status: StatusUnknown}
	parent, err := h.GetParent(ctx)
	if err != nil {
		return info, errors.Wrapf(err, "retrieving parent of host '%s'", h.Id)
	}

	container, err := m.client.GetContainer(ctx, parent, h.Id)
	if err != nil {
		if client.IsErrConnectionFailed(err) {
			info.Status = StatusTerminated
			return info, nil
		}
		if client.IsErrNotFound(err) {
			info.Status = StatusNonExistent
			return info, nil
		}
		return info, errors.Wrapf(err, "getting container information for host '%s'", h.Id)
	}
	info.Status = toEvgStatus(container.State)
	if container.State.Error != "" {
		info.StateReason = container.State.Error
	} else if container.State.OOMKilled {
		info.StateReason = "Out of memory"
	}
	return info, nil
}

func (m *dockerManager) SetPortMappings(ctx context.Context, h, parent *host.Host) error {
	container, err := m.client.GetContainer(ctx, parent, h.Id)
	if err != nil {
		if client.IsErrConnectionFailed(err) {
			return errors.Wrapf(err, "making connection")
		}
		return errors.Wrapf(err, "getting container information for host '%s'", h.Id)
	}
	if !container.State.Running {
		return errors.Errorf("host '%s' is not running", h.Id)

	}

	if err = h.SetPortMapping(ctx, host.GetPortMap(container.NetworkSettings.Ports)); err != nil {
		return errors.Wrapf(err, "saving ports to host '%s", h.Id)
	}
	return nil
}

// GetDNSName does nothing, returning an empty string and no error.
func (m *dockerManager) GetDNSName(ctx context.Context, h *host.Host) (string, error) {
	return "", nil
}

// TerminateInstance destroys a container.
func (m *dockerManager) TerminateInstance(ctx context.Context, h *host.Host, user, reason string) error {
	if h.Status == evergreen.HostTerminated {
		return errors.Errorf("cannot terminate host '%s' because it's already marked as terminated", h.Id)
	}

	parent, err := h.GetParent(ctx)
	if err != nil {
		return errors.Wrapf(err, "retrieving parent for host '%s'", h.Id)
	}

	if err := m.client.RemoveContainer(ctx, parent, h.Id); err != nil {
		return errors.Wrap(err, "removing container")
	}

	grip.Info(message.Fields{
		"message":   "terminated Docker container",
		"container": h.Id,
	})

	// Set the host status as terminated and update its termination time
	return h.Terminate(ctx, user, reason)
}

func (m *dockerManager) StopInstance(ctx context.Context, host *host.Host, shouldKeepOff bool, user string) error {
	return errors.New("StopInstance is not supported for Docker provider")
}

func (m *dockerManager) StartInstance(ctx context.Context, host *host.Host, user string) error {
	return errors.New("StartInstance is not supported for Docker provider")
}

// Configure populates a dockerManager by reading relevant settings from the
// config object.
func (m *dockerManager) Configure(ctx context.Context, s *evergreen.Settings) error {
	config := s.Providers.Docker

	if m.client == nil {
		m.client = GetDockerClient(s)
	}

	if err := m.client.Init(config.APIVersion); err != nil {
		return errors.Wrap(err, "initializing Docker client connection")
	}

	if m.env == nil {
		return errors.New("Docker manager requires a non-nil Evergreen environment")
	}

	return nil
}

func (m *dockerManager) AllocateIP(context.Context) (*host.IPAddress, error) {
	return nil, errors.New("can't allocate IP with Docker provider")
}

func (m *dockerManager) AssociateIP(context.Context, *host.Host) error {
	return errors.New("can't associate IP with Docker provider")
}

func (m *dockerManager) CleanupIP(context.Context, *host.Host) error {
	return nil
}

// Cleanup is a noop for the docker provider.
func (m *dockerManager) Cleanup(context.Context) error {
	return nil
}

func (m *dockerManager) AttachVolume(context.Context, *host.Host, *host.VolumeAttachment) error {
	return errors.New("can't attach volume with Docker provider")
}

func (m *dockerManager) DetachVolume(context.Context, *host.Host, string) error {
	return errors.New("can't detach volume with Docker provider")
}

func (m *dockerManager) CreateVolume(context.Context, *host.Volume) (*host.Volume, error) {
	return nil, errors.New("can't create volume with Docker provider")
}

func (m *dockerManager) DeleteVolume(context.Context, *host.Volume) error {
	return errors.New("can't delete volume with Docker provider")
}

func (m *dockerManager) ModifyVolume(context.Context, *host.Volume, *model.VolumeModifyOptions) error {
	return errors.New("can't modify volume with Docker provider")
}

func (m *dockerManager) GetVolumeAttachment(context.Context, string) (*VolumeAttachment, error) {
	return nil, errors.New("can't get volume attachment with Docker provider")
}

func (m *dockerManager) CheckInstanceType(context.Context, string) error {
	return errors.New("can't specify instance type with Docker provider")
}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host. For Docker this is not relevant.
func (m *dockerManager) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}

func (m *dockerManager) GetContainers(ctx context.Context, h *host.Host) ([]string, error) {
	containers, err := m.client.ListContainers(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "listing containers")
	}

	ids := []string{}
	for _, container := range containers {
		name := container.Names[0]
		// names in Docker have leading slashes -- https://github.com/moby/moby/issues/6705
		if !strings.HasPrefix(name, "/") {
			return nil, errors.New("container name should have leading slash")
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
		return false, errors.Wrap(err, "listing containers")
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
		return errors.Wrap(err, "listing images")
	}

	for i := len(images) - 1; i >= 0; i-- {
		id := images[i].ID
		canBeRemoved, err := m.canImageBeRemoved(ctx, h, id)
		if err != nil {
			return errors.Wrapf(err, "checking whether containers are running on image '%s'", id)
		}
		// remove image based on ID only if there are no containers running the image
		if canBeRemoved {
			err = m.client.RemoveImage(ctx, h, id)
			if err != nil {
				return errors.Wrapf(err, "removing image '%s'", id)
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
		return 0, errors.Wrap(err, "listing images")
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
		return errors.Errorf("host '%s' is not a container parent", parent.Id)
	}

	// Import correct base image if not already on host.
	image, err := m.client.EnsureImageDownloaded(ctx, parent, options)
	if err != nil {
		return errors.Wrapf(err, "ensuring that image '%s' is downloaded on host '%s'", options.Image, parent.Id)
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
	_, err = m.client.BuildImageWithAgent(ctx, m.env.ClientConfig().S3URLPrefix, parent, image)
	if err != nil {
		return errors.Wrapf(err, "building image '%s' with agent on host '%s'", options.Image, parent.Id)
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
