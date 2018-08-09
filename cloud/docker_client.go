// +build go1.7

package cloud

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
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

	Create(*host.Host) error
	Close()

	EnsureImageDownloaded(context.Context, string) (string, error)
	BuildImageWithAgent(context.Context, *host.Host, string) (string, error)
	CreateContainer(context.Context, *host.Host, *dockerSettings) error
	GetContainer(context.Context, string) (*types.ContainerJSON, error)
	ListContainers(context.Context) ([]types.Container, error)
	RemoveImage(context.Context, string) error
	RemoveContainer(context.Context, string) error
	StartContainer(context.Context, string) error
	ListImages(context.Context) ([]types.ImageSummary, error)
}

type dockerClientImpl struct {
	// apiVersion specifies the version of the Docker API.
	apiVersion string
	// httpDockerClient for making HTTP requests within the Docker dockerClient wrapper.
	daemonHost        *host.Host
	httpClient        *http.Client
	client            *docker.Client
	evergreenSettings *evergreen.Settings
}

// template string for new images with agent
const (
	provisionedImageTag = "%s:provisioned"
	imageImportTimeout  = 10 * time.Minute
)

// Init sets the Docker API version to use for API calls to the Docker client.
func (c *dockerClientImpl) Init(apiVersion string) error {
	if apiVersion == "" {
		return errors.Errorf("Docker API version '%s' is invalid", apiVersion)
	}
	c.apiVersion = apiVersion

	return nil
}

// Create generates a Docker client that can talk to the specified host
// machine.
func (c *dockerClientImpl) Create(h *host.Host) error {
	// Do nothing if httpClient and dockerClient already generated
	if c.client != nil && c.httpClient != nil {
		return nil
	}

	// Validate Docker daemon host
	if h.Host == "" {
		return errors.New("HostIP must not be blank")
	}
	c.daemonHost = h

	// Get new httpClient if dockerClient has none
	if c.httpClient == nil {
		err := c.generateHTTPClient()
		if err != nil {
			return errors.Wrap(err, "Error getting new HTTP client")
		}
	}

	// Get new docker.Client
	err := c.generateDockerClient()
	if err != nil {
		return errors.Wrap(err, "Error getting new Docker client")
	}

	return nil
}

// Close puts any HTTP client associated with the Docker client back in the pool.
func (c *dockerClientImpl) Close() {
	if c.httpClient != nil {
		util.PutHTTPClient(c.httpClient)
		c.httpClient = nil
	}
	return
}

// changeTimeout changes the timeout of dockerClient's internal httpClient and
// generates a new docker.Client with the updated timeout.
func (c *dockerClientImpl) changeTimeout(newTimeout time.Duration) error {
	if c.httpClient == nil {
		return errors.New("Error changing timeout: httpClient cannot be nil")
	}

	c.httpClient.Timeout = newTimeout
	err := c.generateDockerClient()
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	return nil
}

// generateHTTPClient gets a new HTTP client from the pool capable of communicating
// over TLS with a remote Docker daemon
func (c *dockerClientImpl) generateHTTPClient() error {
	// Create HTTP client
	c.httpClient = util.GetHTTPClient()

	// Allow connections to Docker daemon with self-signed certificates
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		return errors.Errorf("Type assertion failed: type %T does not hold a *http.Transport", c.httpClient.Transport)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	return nil
}

// generateDockerClient generates a Docker client to wrap Docker API calls. The
// Docker TCP endpoint must be exposed and available for requests at the client
// port on the Docker daemon host machine.
func (c *dockerClientImpl) generateDockerClient() error {
	var err error
	endpoint := fmt.Sprintf("tcp://%s:%v", c.daemonHost.Host, c.daemonHost.ContainerPoolSettings.Port)
	c.client, err = docker.NewClient(endpoint, c.apiVersion, c.httpClient, nil)
	if err != nil {
		grip.Error(message.Fields{
			"message":     "Docker initialize client API call failed",
			"error":       err,
			"endpoint":    endpoint,
			"api_version": c.apiVersion,
		})
		return errors.Wrapf(err, "Docker initialize client API call failed at endpoint '%s'", endpoint)
	}

	return nil
}

// EnsureImageDownloaded checks if the image in s3 specified by the URL already exists,
// and if not, creates a new image from the remote tarball.
func (c *dockerClientImpl) EnsureImageDownloaded(ctx context.Context, url string) (string, error) {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return "", errors.Wrap(err, "Error generating Docker client")
	}

	// Extract image name from url
	baseName := path.Base(url)
	imageName := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Check if image already exists on host
	_, _, err = c.client.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		// Image already exists
		return imageName, nil
	} else if strings.Contains(err.Error(), "No such image") {

		// Extend http client timeout for ImageImport
		normalTimeout := c.httpClient.Timeout
		err = c.changeTimeout(imageImportTimeout)
		if err != nil {
			return "", errors.Wrap(err, "Error changing http client timeout")
		}

		// Image does not exist, import from remote tarball
		source := types.ImageImportSource{SourceName: url}
		msg := makeDockerLogMessage("ImageImport", c.daemonHost.Id, message.Fields{
			"source":     source,
			"image_name": imageName,
			"image_url":  url,
		})
		resp, err := c.client.ImageImport(ctx, source, imageName, types.ImageImportOptions{})
		if err != nil {
			return "", errors.Wrapf(err, "Error importing image from %s", url)
		}
		grip.Info(msg)

		// Wait until ImageImport finishes
		_, err = ioutil.ReadAll(resp)
		if err != nil {
			return "", errors.Wrap(err, "Error reading ImageImport response")
		}

		// Reset http client timeout
		err = c.changeTimeout(normalTimeout)
		if err != nil {
			return "", errors.Wrap(err, "Error changing http client timeout")
		}

		return imageName, nil
	} else {
		return "", errors.Wrapf(err, "Error inspecting image %s", imageName)
	}
}

// BuildImageWithAgent takes a base image and builds a new image on the specified
// host from a Dockfile in the root directory, which adds the Evergreen binary
func (c *dockerClientImpl) BuildImageWithAgent(ctx context.Context, containerHost *host.Host, baseImage string) (string, error) {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return "", errors.Wrap(err, "Error generating Docker client")
	}

	const dockerfileRoute = "dockerfile"

	// modify tag for new image
	provisionedImage := fmt.Sprintf(provisionedImageTag, baseImage)

	executableSubPath := containerHost.Distro.ExecutableSubPath()
	binaryName := containerHost.Distro.BinaryName()

	// build dockerfile route
	dockerfileUrl := strings.Join([]string{
		c.evergreenSettings.ApiUrl,
		evergreen.APIRoutePrefix,
		dockerfileRoute,
	}, "/")

	options := types.ImageBuildOptions{
		BuildArgs: map[string]*string{
			"BASE_IMAGE":          &baseImage,
			"EXECUTABLE_SUB_PATH": &executableSubPath,
			"BINARY_NAME":         &binaryName,
			"URL":                 &c.evergreenSettings.Ui.Url,
		},
		Remove:        true,
		RemoteContext: dockerfileUrl,
		Tags:          []string{provisionedImage},
	}

	msg := makeDockerLogMessage("ImageBuild", c.daemonHost.Id, message.Fields{
		"base_image":     options.BuildArgs["BASE_IMAGE"],
		"dockerfile_url": options.RemoteContext,
	})

	// build the image
	resp, err := c.client.ImageBuild(ctx, nil, options)
	if err != nil {
		return "", errors.Wrapf(err, "Error building Docker image from base image %s", baseImage)
	}
	grip.Info(msg)

	// wait for ImageBuild to complete -- success response otherwise returned
	// before building from Dockerfile is over, and next ContainerCreate will fail
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "Error reading ImageBuild response")
	}

	return provisionedImage, nil
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
func (c *dockerClientImpl) CreateContainer(ctx context.Context, containerHost *host.Host, settings *dockerSettings) error {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return errors.Wrap(err, "Error generating Docker client")
	}

	// Import correct base image if not already on host.
	image, err := c.EnsureImageDownloaded(ctx, settings.ImageURL)
	if err != nil {
		return errors.Wrapf(err, "Unable to ensure that image '%s' is on host '%s'", settings.ImageURL, c.daemonHost.Id)
	}

	// Build image containing Evergreen executable.
	provisionedImage, err := c.BuildImageWithAgent(ctx, containerHost, image)
	if err != nil {
		return errors.Wrapf(err, "Failed to build image %s with agent on host '%s'", image, c.daemonHost.Id)
	}

	// Build path to Evergreen executable.
	pathToExecutable := filepath.Join("root", "evergreen")
	if containerHost.Distro.IsWindows() {
		pathToExecutable += ".exe"
	}

	// Generate the host secret for container if none exists.
	if containerHost.Secret == "" {
		if err = containerHost.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", containerHost.Id)
		}
	}

	// Build Evergreen agent command.
	agentCmdParts := []string{
		pathToExecutable,
		"agent",
		fmt.Sprintf("--api_server=%s", c.evergreenSettings.ApiUrl),
		fmt.Sprintf("--host_id=%s", containerHost.Id),
		fmt.Sprintf("--host_secret=%s", containerHost.Secret),
		fmt.Sprintf("--log_prefix=%s", filepath.Join(containerHost.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory=%s", containerHost.Distro.WorkDir),
		"--cleanup",
	}

	// Populate container settings with command and new image.
	containerConf := &container.Config{
		Cmd:   agentCmdParts,
		Image: provisionedImage,
		User:  containerHost.Distro.User,
	}
	networkConf := &network.NetworkingConfig{}
	hostConf := &container.HostConfig{}

	msg := makeDockerLogMessage("ContainerCreate", c.daemonHost.Id, message.Fields{
		"image": containerConf.Image,
	})

	// Build container
	if _, err := c.client.ContainerCreate(ctx, containerConf, hostConf, networkConf, containerHost.Id); err != nil {
		err = errors.Wrapf(err, "Docker create API call failed for container '%s'", containerHost.Id)
		grip.Error(err)
		return err
	}
	grip.Info(msg)

	return nil
}

// GetContainer returns low-level information on the Docker container with the
// specified ID running on the specified host machine.
func (c *dockerClientImpl) GetContainer(ctx context.Context, containerID string) (*types.ContainerJSON, error) {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return nil, errors.Wrap(err, "Error generating Docker client")
	}

	container, err := c.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, errors.Wrapf(err, "Docker inspect API call failed for container '%s'", containerID)
	}

	return &container, nil
}

// ListContainers lists all containers running on the specified host machine.
func (c *dockerClientImpl) ListContainers(ctx context.Context) ([]types.Container, error) {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return nil, errors.Wrap(err, "Error generating Docker client")
	}

	// Get all running containers
	opts := types.ContainerListOptions{All: false}
	containers, err := c.client.ContainerList(ctx, opts)
	if err != nil {
		err = errors.Wrap(err, "Docker list API call failed")
		grip.Error(err)
		return nil, err
	}

	return containers, nil
}

// ListImages lists all images on the specified host machine.
func (c *dockerClientImpl) ListImages(ctx context.Context) ([]types.ImageSummary, error) {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return nil, errors.Wrap(err, "Error generating Docker client")
	}

	// Get all container images
	opts := types.ImageListOptions{All: false}
	images, err := c.client.ImageList(ctx, opts)
	if err != nil {
		err = errors.Wrap(err, "Docker list API call failed")
		return nil, err
	}

	return images, nil
}

// RemoveImage forcibly removes an image from its host machine
func (c *dockerClientImpl) RemoveImage(ctx context.Context, imageID string) error {
	// Check that Docker client is properly configured
	err := c.Create(c.daemonHost)
	if err != nil {
		return errors.Wrap(err, "Error generating Docker client")
	}

	opts := types.ImageRemoveOptions{Force: true}
	removed, err := c.client.ImageRemove(ctx, imageID, opts)
	if err != nil {
		err = errors.Wrapf(err, "Failed to remove image '%s'", imageID)
		return err
	}
	// check to make sure an image was removed
	if len(removed) <= 0 {
		return errors.Errorf("Failed to remove image '%s'", imageID)
	}
	return nil
}

// RemoveContainer forcibly removes a running or stopped container by ID from its host machine.
func (c *dockerClientImpl) RemoveContainer(ctx context.Context, containerID string) error {
	err := c.Create(c.daemonHost)
	if err != nil {
		return errors.Wrap(err, "Error generating Docker client")
	}

	opts := types.ContainerRemoveOptions{Force: true}
	if err := c.client.ContainerRemove(ctx, containerID, opts); err != nil {
		err = errors.Wrapf(err, "Failed to remove container '%s'", containerID)
		grip.Error(err)
		return err
	}

	return nil
}

// StartContainer starts a stopped or new container by ID on the host machine.
func (c *dockerClientImpl) StartContainer(ctx context.Context, containerID string) error {
	err := c.Create(c.daemonHost)
	if err != nil {
		return errors.Wrap(err, "Error generating Docker client")
	}

	opts := types.ContainerStartOptions{}
	if err := c.client.ContainerStart(ctx, containerID, opts); err != nil {
		return errors.Wrapf(err, "Failed to start container %s", containerID)
	}

	return nil
}

func makeDockerLogMessage(name, parent string, data interface{}) message.Fields {
	return message.Fields{
		"message":  "Docker API call",
		"api_name": name,
		"parent":   parent,
		"data":     data,
	}
}
