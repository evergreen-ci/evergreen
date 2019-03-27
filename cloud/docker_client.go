// +build go1.7

package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// The DockerClient interface wraps the Docker dockerClient interaction.
type DockerClient interface {
	Init(string) error
	EnsureImageDownloaded(context.Context, *host.Host, host.DockerOptions) (string, error)
	BuildImageWithAgent(context.Context, *host.Host, string) (string, error)
	CreateContainer(context.Context, *host.Host, *host.Host) error
	GetContainer(context.Context, *host.Host, string) (*types.ContainerJSON, error)
	GetDockerLogs(context.Context, string, *host.Host, types.ContainerLogsOptions) (io.Reader, error)
	GetDockerStatus(context.Context, string, *host.Host) (*ContainerStatus, error)
	ListContainers(context.Context, *host.Host) ([]types.Container, error)
	RemoveImage(context.Context, *host.Host, string) error
	RemoveContainer(context.Context, *host.Host, string) error
	StartContainer(context.Context, *host.Host, string) error
	ListImages(context.Context, *host.Host) ([]types.ImageSummary, error)
}

type dockerClientImpl struct {
	// apiVersion specifies the version of the Docker API.
	apiVersion string
	// httpDockerClient for making HTTP requests within the Docker dockerClient wrapper.
	httpClient        *http.Client
	client            *docker.Client
	evergreenSettings *evergreen.Settings
}

type ContainerStatus struct {
	IsRunning  bool
	HasStarted bool
}

// template string for new images with agent
const (
	provisionedImageTag = "%s:provisioned"
	imageImportTimeout  = 10 * time.Minute
)

func GetDockerClient(s *evergreen.Settings) DockerClient {
	var client DockerClient = &dockerClientImpl{evergreenSettings: s}
	return client
}

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

// changeTimeout changes the timeout of dockerClient's internal httpClient and
// returns a new docker.Client with the updated timeout
func (c *dockerClientImpl) changeTimeout(h *host.Host, newTimeout time.Duration) (*docker.Client, error) {
	var err error
	c.httpClient.Timeout = newTimeout
	c.client, err = c.generateClient(h)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
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

// EnsureImageDownloaded checks if the image in s3 specified by the URL already exists,
// and if not, creates a new image from the remote tarball.
func (c *dockerClientImpl) EnsureImageDownloaded(ctx context.Context, h *host.Host, options host.DockerOptions) (string, error) {
	start := time.Now()
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return "", errors.Wrap(err, "Failed to generate docker client")
	}

	// Extract image name from url
	baseName := path.Base(options.Image)
	imageName := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Check if image already exists on host
	_, _, err = dockerClient.ImageInspectWithRaw(ctx, imageName)
	grip.Info(message.Fields{
		"operation":     "EnsureImageDownloaded",
		"details":       "ImageInspectWithRaw",
		"duration_secs": time.Since(start).Seconds(),
	})
	if err == nil {
		// Image already exists
		return imageName, nil
	} else if strings.Contains(err.Error(), "No such image") {
		if options.Method == distro.DockerImageBuildTypeImport {
			err = c.importImage(ctx, h, imageName, options.Image)
			grip.Info(message.Fields{
				"operation":     "EnsureImageDownloaded",
				"details":       "import image",
				"duration_secs": time.Since(start).Seconds(),
			})
			return imageName, errors.Wrap(err, "error importing image")
		} else if options.Method == distro.DockerImageBuildTypePull {
			image := options.Image
			if options.RegistryName != "" {
				image = fmt.Sprintf("%s/%s", options.RegistryName, imageName)
			}
			err = c.pullImage(ctx, h, image, options.RegistryUsername, options.RegistryPassword)
			grip.Info(message.Fields{
				"operation":     "EnsureImageDownloaded",
				"details":       "pull image",
				"duration_secs": time.Since(start).Seconds(),
			})
			return imageName, errors.Wrap(err, "error pulling image")
		}
		return imageName, errors.Errorf("unrecognized image build method: %s", options.Method)
	}
	return "", errors.Wrapf(err, "Error inspecting image %s", imageName)
}

func (c *dockerClientImpl) importImage(ctx context.Context, h *host.Host, name, url string) error {
	// Extend http client timeout for ImageImport
	normalTimeout := c.httpClient.Timeout
	dockerClient, err := c.changeTimeout(h, imageImportTimeout)
	if err != nil {
		return errors.Wrap(err, "Error changing http client timeout")
	}

	// Image does not exist, import from remote tarball
	source := types.ImageImportSource{SourceName: url}
	var resp io.ReadCloser
	resp, err = dockerClient.ImageImport(ctx, source, name, types.ImageImportOptions{})
	if err != nil {
		return errors.Wrapf(err, "Error importing image from %s", url)
	}

	// Wait until ImageImport finishes
	_, err = ioutil.ReadAll(resp)
	if err != nil {
		return errors.Wrap(err, "Error reading ImageImport response")
	}

	// Reset http client timeout
	_, err = c.changeTimeout(h, normalTimeout)
	return errors.Wrap(err, "Error changing http client timeout")
}

func (c *dockerClientImpl) pullImage(ctx context.Context, h *host.Host, url, username, password string) error {
	normalTimeout := c.httpClient.Timeout
	dockerClient, err := c.changeTimeout(h, imageImportTimeout)
	if err != nil {
		return errors.Wrap(err, "Error changing http client timeout")
	}

	var auth string
	if username != "" {
		authConfig := types.AuthConfig{
			Username: username,
			Password: password,
		}
		var jsonBytes []byte
		jsonBytes, err = json.Marshal(authConfig)
		if err != nil {
			return errors.Wrap(err, "error marshaling auth config")
		}
		auth = base64.URLEncoding.EncodeToString(jsonBytes)
	}
	resp, err := dockerClient.ImagePull(ctx, url, types.ImagePullOptions{RegistryAuth: auth})
	if err != nil {
		return errors.Wrap(err, "error pulling image from registry")
	}
	_, err = ioutil.ReadAll(resp)
	if err != nil {
		return errors.Wrap(err, "error reading image pull response")
	}
	_, err = c.changeTimeout(h, normalTimeout)
	return errors.Wrap(err, "Error changing http client timeout")
}

// BuildImageWithAgent takes a base image and builds a new image on the specified
// host from a Dockfile in the root directory, which adds the Evergreen binary
func (c *dockerClientImpl) BuildImageWithAgent(ctx context.Context, h *host.Host, baseImage string) (string, error) {
	const dockerfileRoute = "dockerfile"
	start := time.Now()

	dockerClient, err := c.generateClient(h)
	if err != nil {
		return "", errors.Wrap(err, "Failed to generate docker client")
	}
	grip.Info(message.Fields{
		"operation": "BuildImageWithAgent",
		"details":   "generateclient",
		"duration":  time.Since(start),
		"span":      time.Since(start).String(),
	})

	// modify tag for new image
	provisionedImage := fmt.Sprintf(provisionedImageTag, baseImage)

	executableSubPath := h.Distro.ExecutableSubPath()
	binaryName := h.Distro.BinaryName()

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

	msg := makeDockerLogMessage("ImageBuild", h.Id, message.Fields{
		"base_image":     options.BuildArgs["BASE_IMAGE"],
		"dockerfile_url": options.RemoteContext,
	})

	// build the image
	resp, err := dockerClient.ImageBuild(ctx, nil, options)
	if err != nil {
		return "", errors.Wrapf(err, "Error building Docker image from base image %s", baseImage)
	}
	grip.Info(message.Fields{
		"operation": "BuildImageWithAgent",
		"details":   "ImageBuild",
		"duration":  time.Since(start),
		"span":      time.Since(start).String(),
	})
	grip.Info(msg)

	// wait for ImageBuild to complete -- success response otherwise returned
	// before building from Dockerfile is over, and next ContainerCreate will fail
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "Error reading ImageBuild response")
	}
	grip.Info(message.Fields{
		"operation": "BuildImageWithAgent",
		"details":   "ReadAll",
		"duration":  time.Since(start),
		"span":      time.Since(start).String(),
	})

	return provisionedImage, nil
}

// CreateContainer creates a new Docker container with Evergreen agent.
func (c *dockerClientImpl) CreateContainer(ctx context.Context, parentHost, containerHost *host.Host) error {
	dockerClient, err := c.generateClient(parentHost)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	// Extract image name from url
	baseName := path.Base(containerHost.DockerOptions.Image)
	provisionedImage := fmt.Sprintf(provisionedImageTag, strings.TrimSuffix(baseName, filepath.Ext(baseName)))

	agentCmdParts := []string{containerHost.DockerOptions.Command}
	if containerHost.DockerOptions.Command == "" {
		// Generate the host secret for container if none exists.
		if containerHost.Secret == "" {
			if err = containerHost.CreateSecret(); err != nil {
				return errors.Wrapf(err, "creating secret for %s", containerHost.Id)
			}
		}
		// Build path to Evergreen executable.
		pathToExecutable := filepath.Join("/", "evergreen")
		if parentHost.Distro.IsWindows() {
			pathToExecutable += ".exe"
		}
		// Build Evergreen agent command.
		agentCmdParts = []string{
			pathToExecutable,
			"agent",
			fmt.Sprintf("--api_server=%s", c.evergreenSettings.ApiUrl),
			fmt.Sprintf("--host_id=%s", containerHost.Id),
			fmt.Sprintf("--host_secret=%s", containerHost.Secret),
			fmt.Sprintf("--log_prefix=%s", filepath.Join(containerHost.Distro.WorkDir, "agent")),
			fmt.Sprintf("--working_directory=%s", containerHost.Distro.WorkDir),
			"--cleanup",
		}
		containerHost.DockerOptions.Command = strings.Join(agentCmdParts, "\n")
	}

	// Populate container settings with command and new image.
	containerConf := &container.Config{
		Cmd:   agentCmdParts,
		Image: provisionedImage,
		User:  containerHost.Distro.User,
	}
	networkConf := &network.NetworkingConfig{}
	hostConf := &container.HostConfig{}

	msg := makeDockerLogMessage("ContainerCreate", parentHost.Id, message.Fields{
		"image": containerConf.Image,
	})

	// Build container
	info, err := dockerClient.ContainerCreate(ctx, containerConf, hostConf, networkConf, containerHost.Id)
	if err != nil {
		err = errors.Wrapf(err, "Docker create API call failed for container '%s'", containerHost.Id)
		grip.Error(err)
		return err
	}
	containerHost.ExternalIdentifier = info.ID
	grip.Info(msg)

	return nil
}

// GetDockerLogs returns output logs or error logs, based on the given options.
// This assumes the container is not using TTY.
func (c *dockerClientImpl) GetDockerLogs(ctx context.Context, containerID string, parent *host.Host, options types.ContainerLogsOptions) (io.Reader, error) {
	dockerClient, err := c.generateClient(parent)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	if containerID == "" { // container not started yet
		return nil, errors.New("container has not started")
	}
	stream, err := dockerClient.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return nil, errors.Wrapf(err, "Docker logs API call failed for container %s", containerID)
	}
	tempout := &bytes.Buffer{}
	temperr := &bytes.Buffer{}

	_, err = stdcopy.StdCopy(tempout, temperr, stream)
	if err != nil {
		return nil, errors.Wrapf(err, "Error copying stream for container %s", containerID)
	}

	if options.ShowStdout {
		return tempout, nil
	}
	return temperr, nil
}

func (c *dockerClientImpl) GetDockerStatus(ctx context.Context, containerID string, parent *host.Host) (*ContainerStatus, error) {
	if containerID == "" {
		return &ContainerStatus{HasStarted: false}, nil
	}
	status := ContainerStatus{HasStarted: true}
	container, err := c.GetContainer(ctx, parent, containerID)
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting container %s", containerID)
	}
	if container == nil {
		return nil, errors.Errorf("Container %s returned empty", containerID)
	}

	status.IsRunning = container.State.Running
	return &status, nil
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

// ListImages lists all images on the specified host machine.
func (c *dockerClientImpl) ListImages(ctx context.Context, h *host.Host) ([]types.ImageSummary, error) {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate docker client")
	}

	// Get all container images
	opts := types.ImageListOptions{All: false}
	images, err := dockerClient.ImageList(ctx, opts)
	if err != nil {
		err = errors.Wrap(err, "Docker list API call failed")
		return nil, err
	}

	return images, nil
}

// RemoveImage forcibly removes an image from its host machine
func (c *dockerClientImpl) RemoveImage(ctx context.Context, h *host.Host, imageID string) error {
	dockerClient, err := c.generateClient(h)
	if err != nil {
		return errors.Wrap(err, "Failed to generate docker client")
	}

	opts := types.ImageRemoveOptions{Force: true}
	removed, err := dockerClient.ImageRemove(ctx, imageID, opts)
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

func makeDockerLogMessage(name, parent string, data interface{}) message.Fields {
	return message.Fields{
		"message":  "Docker API call",
		"api_name": name,
		"parent":   parent,
		"data":     data,
	}
}
