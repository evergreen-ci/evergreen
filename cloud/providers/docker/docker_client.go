// +build go1.7

package docker

import (
	"context"
	"fmt"
	"net/http"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// The client interface wraps the Docker client interaction.
type client interface {
	Init(string) error
	CreateContainer(string, *distro.Distro, *ProviderSettings) error
	GetContainer(*host.Host) (*types.ContainerJSON, error)
	ListContainers(*distro.Distro) ([]types.Container, error)
	RemoveContainer(*host.Host) error
	StartContainer(*host.Host) error
}

type clientImpl struct {
	// apiVersion specifies the version of the Docker API.
	apiVersion string
	// httpClient for making HTTP requests within the Docker client wrapper.
	httpClient *http.Client
}

// generateClient generates a Docker client that can talk to the host machine
// specified in the distro. The Docker client must be exposed and available for
// requests at the distro-specified client port on the host machine.
func (c *clientImpl) generateClient(d *distro.Distro) (*docker.Client, error) {
	// Populate and validate settings
	settings := &ProviderSettings{} // Instantiate global settings
	if err := mapstructure.Decode(d.ProviderSettings, settings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro '%s'", d.Id)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid Docker settings in distro '%s'", d.Id)
	}

	// Create a Docker client to wrap Docker API calls. The Docker TCP endpoint must
	// be exposed and available for requests at the client port on the host machine.
	endpoint := fmt.Sprintf("tcp://%s:%v", settings.HostIP, settings.ClientPort)
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
func (c *clientImpl) Init(apiVersion string) error {
	if apiVersion == "" {
		return errors.Errorf("Docker API version '%s' is invalid", apiVersion)
	}
	c.apiVersion = apiVersion

	// Create HTTP client
	c.httpClient = util.GetHttpClient()
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
func (c *clientImpl) CreateContainer(id string, d *distro.Distro, s *ProviderSettings) error {
	dockerClient, err := c.generateClient(d)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	// List all containers to find ports that are already taken.
	containers, err := c.ListContainers(d)
	if err != nil {
		return errors.Wrapf(err, "Failed to list containers for distro '%s'", d.Id)
	}

	// Create a host config to bind the SSH port to another open port.
	hostConf, err := makeHostConfig(d, s, containers)
	if err != nil {
		err = errors.Wrapf(err, "Unable to populate docker host config for host '%s'", s.HostIP)
		grip.Error(err)
		return err
	}

	// Populate container settings.
	ctx := context.TODO()
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
		"name":          id,
		"image_id":      containerConf.Image,
		"exposed_ports": containerConf.ExposedPorts,
		"port_bindings": hostConf.PortBindings,
	})

	// Build container
	if _, err := dockerClient.ContainerCreate(ctx, containerConf, hostConf, networkConf, id); err != nil {
		err = errors.Wrapf(err, "Docker create API call failed for container '%s'", id)
		grip.Error(err)
		return err
	}

	return nil
}

// GetContainer returns low-level information on the Docker container.
func (c *clientImpl) GetContainer(h *host.Host) (*types.ContainerJSON, error) {
	dockerClient, err := c.generateClient(&h.Distro)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	ctx := context.TODO()
	container, err := dockerClient.ContainerInspect(ctx, h.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "Docker inspect API call failed for container '%s'", h.Id)
	}

	return &container, nil
}

// ListContainers lists all containers running on the distro-specified host machine.
func (c *clientImpl) ListContainers(d *distro.Distro) ([]types.Container, error) {
	dockerClient, err := c.generateClient(d)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	// Get all the things!
	ctx := context.TODO()
	opts := types.ContainerListOptions{All: true}
	containers, err := dockerClient.ContainerList(ctx, opts)
	if err != nil {
		err = errors.Wrap(err, "Docker list API call failed")
		grip.Error(err)
		return nil, err
	}

	return containers, nil
}

// RemoveContainer forcibly removes a running or stopped container from its host machine.
func (c *clientImpl) RemoveContainer(h *host.Host) error {
	dockerClient, err := c.generateClient(&h.Distro)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	ctx := context.TODO()
	opts := types.ContainerRemoveOptions{Force: true}
	if err = dockerClient.ContainerRemove(ctx, h.Id, opts); err != nil {
		err = errors.Wrapf(err, "Failed to remove container '%s'", h.Id)
		grip.Error(err)
		return err
	}

	return nil
}

// StartContainer starts a stopped or new container on the host machine.
func (c *clientImpl) StartContainer(h *host.Host) error {
	dockerClient, err := c.generateClient(&h.Distro)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	ctx := context.TODO()
	opts := types.ContainerStartOptions{}
	if err := dockerClient.ContainerStart(ctx, h.Id, opts); err != nil {
		return errors.Wrapf(err, "Failed to start container %s", h.Id)
	}

	return nil
}
