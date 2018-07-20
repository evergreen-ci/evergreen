// +build go1.7

package cloud

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// The dockerClient interface wraps the Docker dockerClient interaction.
type dockerClient interface {
	Init(string) error
	BuildImageWithAgent(context.Context, *host.Host, string) (string, error)
	CreateContainer(context.Context, *host.Host, string, *dockerSettings) error
	GetContainer(context.Context, *host.Host, string) (*types.ContainerJSON, error)
	ListContainers(context.Context, *host.Host) ([]types.Container, error)
	RemoveContainer(context.Context, *host.Host, string) error
	StartContainer(context.Context, *host.Host, string) error
}

type dockerClientImpl struct {
	// apiVersion specifies the version of the Docker API.
	apiVersion string
	// httpDockerClient for making HTTP requests within the Docker dockerClient wrapper.
	httpClient        *http.Client
	client            *docker.Client
	evergreenSettings *evergreen.Settings
}

// template string for new images with agent
const newImageName string = "%s-agent"

// generateClient generates a Docker client that can talk to the specified host
// machine. The Docker client must be exposed and available for requests at the
// client port 3369 on the host machine.
func (c *dockerClientImpl) generateClient(h *host.Host) (*docker.Client, error) {
	if h.Host == "" {
		return nil, errors.New("HostIP must not be blank")
	}

	// cache the *docker.Client in dockerClientImpl
	if c.client != nil {
		return c.client, nil
	}

	// Create a Docker client to wrap Docker API calls. The Docker TCP endpoint must
	// be exposed and available for requests at the client port on the host machine.
	var err error
	endpoint := fmt.Sprintf("tcp://%s:%v", h.Host, h.ContainerPoolSettings.Port)
	c.client, err = docker.NewClient(endpoint, c.apiVersion, c.httpClient, nil)
	if err != nil {
		grip.Error(message.Fields{
			"message":     "Docker initialize client API call failed",
			"error":       err,
			"endpoint":    endpoint,
			"api_version": c.apiVersion,
		})
		return nil, errors.Wrapf(err, "Docker initialize client API call failed at endpoint '%s'", endpoint)
	}

	return c.client, nil
}

// Init sets the Docker API version to use for API calls to the Docker client.
func (c *dockerClientImpl) Init(apiVersion string) error {
	if apiVersion == "" {
		return errors.Errorf("Docker API version '%s' is invalid", apiVersion)
	}
	c.apiVersion = apiVersion

	// Create HTTP client
	c.httpClient = util.GetHTTPClient()

	// allow connections to Docker daemon with self-signed certificates
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		return errors.Errorf("Type assertion failed: type %T does not hold a *http.Transport", c.httpClient.Transport)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	return nil
}

// BuildImageWithAgent takes a base image and builds a new image on the specified
// host from a Dockfile in the root directory, which adds the Evergreen binary
func (c *dockerClientImpl) BuildImageWithAgent(ctx context.Context, h *host.Host, baseImage string) (string, error) {
	const dockerfileRoute = "/dockerfile"

	dockerClient, err := c.generateClient(h)
	if err != nil {
		return "", errors.Wrap(err, "Failed to generate docker client")
	}

	// modify tag for new image
	newImage := fmt.Sprintf(newImageName, baseImage)

	executableSubPath := h.Distro.ExecutableSubPath()
	binaryName := h.Distro.BinaryName()

	// build dockerfile route
	dockerfileUrl := strings.Join([]string{
		c.evergreenSettings.ApiUrl,
		evergreen.APIRoutePrefixV2,
		dockerfileRoute,
	}, "")

	options := types.ImageBuildOptions{
		BuildArgs: map[string]*string{
			"BASE_IMAGE":          &baseImage,
			"EXECUTABLE_SUB_PATH": &executableSubPath,
			"BINARY_NAME":         &binaryName,
			"URL":                 &c.evergreenSettings.Ui.Url,
		},
		Remove:        true,
		RemoteContext: dockerfileUrl,
		Tags:          []string{newImage},
	}

	// build the image
	resp, err := dockerClient.ImageBuild(ctx, nil, options)
	if err != nil {
		return "", errors.Wrapf(err, "Error building Docker image from base image %s", baseImage)
	}

	// wait for ImageBuild to complete -- success response otherwise returned
	// before building from Dockerfile is over, and next ContainerCreate will fail
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "Error reading ImageBuild response")
	}

	return newImage, nil
}

// CreateContainer creates a new Docker container that runs an SSH daemon, and binds the
// container's SSH port to another port on the host machine. The preloaded Docker image
// must satisfy the following constraints:
//     1. The image must have the sshd binary in order to start the SSH daemon. On Ubuntu
//        14.04, you can get it with `apt-get update && apt-get install -y openssh-server`.
//     2. The image must execute the SSH daemon without detaching on startup. On Ubuntu
//        14.04, this command is `/usr/sbin/sshd -D`.
//     3. The image must have the same ~/.ssh/authorized_keys file as the host machine
//        in order to allow users with SSH access to the host machine to have SSH access
//        to the container.
func (c *dockerClientImpl) CreateContainer(ctx context.Context, h *host.Host, name string, ds *dockerSettings) error {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	// List all containers to find ports that are already taken.
	containers, err := c.ListContainers(ctx, h)
	if err != nil {
		return errors.Wrapf(err, "Failed to list containers on host '%s'", h.Id)
	}

	// Create a host config to bind the SSH port to another open port.
	hostConf, err := makeHostConfig(h, containers)
	if err != nil {
		err = errors.Wrapf(err, "Unable to populate docker host config for host '%s'", h.Host)
		grip.Error(err)
		return err
	}

	// Build image containing Evergreen executable.
	newImage, err := c.BuildImageWithAgent(ctx, h, ds.ImageID)
	if err != nil {
		return errors.Wrapf(err, "Failed to build image %s with agent on host '%s'", ds.ImageID, h.Id)
	}

	// Build path to Evergreen executable.
	pathToExecutable := filepath.Join("root", "evergreen")
	if h.Distro.IsWindows() {
		pathToExecutable += ".exe"
	}

	// Generate the host secret for container if none exists.
	if h.Secret == "" {
		if err = h.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", h.Id)
		}
	}

	// Build Evergreen agent command.
	agentCmdParts := []string{
		pathToExecutable,
		"agent",
		fmt.Sprintf("--api_server='%s'", c.evergreenSettings.ApiUrl),
		fmt.Sprintf("--host_id='%s'", h.Id),
		fmt.Sprintf("--host_secret='%s'", h.Secret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(h.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory='%s'", h.Distro.WorkDir),
		"--cleanup",
	}

	// Populate container settings with command and new image.
	containerConf := &container.Config{
		// ExposedPorts exposes the default SSH port to external connections.
		ExposedPorts: nat.PortSet{
			sshdPort: {},
		},
		Cmd:   agentCmdParts,
		Image: newImage,
	}
	networkConf := &network.NetworkingConfig{}

	grip.Info(message.Fields{
		"message":       "Creating docker container",
		"name":          name,
		"image_id":      containerConf.Image,
		"exposed_ports": containerConf.ExposedPorts,
		"port_bindings": hostConf.PortBindings,
	})

	// Build container
	if _, err := dockerClient.ContainerCreate(ctx, containerConf, hostConf, networkConf, name); err != nil {
		err = errors.Wrapf(err, "Docker create API call failed for container '%s'", name)
		grip.Error(err)
		return err
	}

	return nil
}

// GetContainer returns low-level information on the Docker container with the
// specified ID running on the specified host machine.
func (c *dockerClientImpl) GetContainer(ctx context.Context, h *host.Host, containerID string) (*types.ContainerJSON, error) {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	container, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, errors.Wrapf(err, "Docker inspect API call failed for container '%s'", containerID)
	}

	return &container, nil
}

// ListContainers lists all containers running on the specified host machine.
func (c *dockerClientImpl) ListContainers(ctx context.Context, h *host.Host) ([]types.Container, error) {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	// Get all running containers
	opts := types.ContainerListOptions{All: false}
	containers, err := dockerClient.ContainerList(ctx, opts)
	if err != nil {
		err = errors.Wrap(err, "Docker list API call failed")
		grip.Error(err)
		return nil, err
	}

	return containers, nil
}

// RemoveContainer forcibly removes a running or stopped container by ID from its host machine.
func (c *dockerClientImpl) RemoveContainer(ctx context.Context, h *host.Host, containerID string) error {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	opts := types.ContainerRemoveOptions{Force: true}
	if err = dockerClient.ContainerRemove(ctx, containerID, opts); err != nil {
		err = errors.Wrapf(err, "Failed to remove container '%s'", containerID)
		grip.Error(err)
		return err
	}

	return nil
}

// StartContainer starts a stopped or new container by ID on the host machine.
func (c *dockerClientImpl) StartContainer(ctx context.Context, h *host.Host, containerID string) error {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	opts := types.ContainerStartOptions{}
	if err := dockerClient.ContainerStart(ctx, containerID, opts); err != nil {
		return errors.Wrapf(err, "Failed to start container %s", containerID)
	}

	return nil
}
