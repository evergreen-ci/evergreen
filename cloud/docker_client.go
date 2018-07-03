// +build go1.7

package cloud

import (
	"context"
	"fmt"
	"net/http"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Temporary port used for all connections to the Docker daemon. To be moved to
// ContainerPoolsConfig section of Evergreen settings after EVG-3598
const ClientPort = 3389

// The dockerClient interface wraps the Docker dockerClient interaction.
type dockerClient interface {
	Init(string) error
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
	httpClient *http.Client
}

// generateClient generates a Docker client that can talk to the specified host
// machine. The Docker client must be exposed and available for requests at the
// client port 3369 on the host machine.
func (c *dockerClientImpl) generateClient(h *host.Host) (*docker.Client, error) {
	if h.Host == "" {
		return nil, errors.New("HostIP must not be blank")
	}

	// Create a Docker client to wrap Docker API calls. The Docker TCP endpoint must
	// be exposed and available for requests at the client port on the host machine.
	endpoint := fmt.Sprintf("tcp://%s:%v", h.Host, ClientPort)
	client, err := docker.NewClient(endpoint, c.apiVersion, c.httpClient, nil)
	if err != nil {
		grip.Error(message.Fields{
			"message":     "Docker initialize client API call failed",
			"error":       err,
			"endpoint":    endpoint,
			"api_version": c.apiVersion,
		})
		return nil, errors.Wrapf(err, "Docker initialize client API call failed at endpoint '%s'", endpoint)
	}

	return client, nil
}

// Init sets the Docker API version to use for API calls to the Docker client.
func (c *dockerClientImpl) Init(apiVersion string) error {
	if apiVersion == "" {
		return errors.Errorf("Docker API version '%s' is invalid", apiVersion)
	}
	c.apiVersion = apiVersion

	// Create HTTP client
	c.httpClient = util.GetHTTPClient()
	return nil
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
func (c *dockerClientImpl) CreateContainer(ctx context.Context, h *host.Host, name string, s *dockerSettings) error {
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
	hostConf, err := makeHostConfig(s, containers)
	if err != nil {
		err = errors.Wrapf(err, "Unable to populate docker host config for host '%s'", s.HostIP)
		grip.Error(err)
		return err
	}

	// Populate container settings.
	containerConf := &container.Config{
		// ExposedPorts exposes the default SSH port to external connections.
		ExposedPorts: nat.PortSet{
			sshdPort: {},
		},
		Image: s.ImageID,
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
func (c *dockerClientImpl) GetContainer(ctx context.Context, h *host.Host, id string) (*types.ContainerJSON, error) {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	container, err := dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "Docker inspect API call failed for container '%s'", id)
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
func (c *dockerClientImpl) RemoveContainer(ctx context.Context, h *host.Host, id string) error {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	opts := types.ContainerRemoveOptions{Force: true}
	if err = dockerClient.ContainerRemove(ctx, id, opts); err != nil {
		err = errors.Wrapf(err, "Failed to remove container '%s'", id)
		grip.Error(err)
		return err
	}

	return nil
}

// StartContainer starts a stopped or new container by ID on the host machine.
func (c *dockerClientImpl) StartContainer(ctx context.Context, h *host.Host, id string) error {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	opts := types.ContainerStartOptions{}
	if err := dockerClient.ContainerStart(ctx, id, opts); err != nil {
		return errors.Wrapf(err, "Failed to start container %s", id)
	}

	return nil
}
