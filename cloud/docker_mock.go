package cloud

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

type dockerClientMock struct {
	// API call options
	failInit     bool
	failDownload bool
	failBuild    bool
	failCreate   bool
	failGet      bool
	failList     bool
	failRemove   bool
	failStart    bool
	failAttach   bool

	// Other options
	hasOpenPorts        bool
	baseImage           string
	containerAttachment *types.HijackedResponse
}

func (c *dockerClientMock) generateContainerID() string {
	return fmt.Sprintf("container-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

func (c *dockerClientMock) Init(string) error {
	if c.failInit {
		return errors.New("failed to initialize client")
	}
	return nil
}

func (c *dockerClientMock) EnsureImageDownloaded(context.Context, *host.Host, host.DockerOptions) (string, error) {
	if c.failDownload {
		return "", errors.New("failed to download image")
	}
	return c.baseImage, nil
}

func (c *dockerClientMock) BuildImageWithAgent(context.Context, string, *host.Host, string) (string, error) {
	if c.failBuild {
		return "", errors.New("failed to build image with agent")
	}
	return fmt.Sprintf(provisionedImageTag, c.baseImage), nil
}

func (c *dockerClientMock) CreateContainer(context.Context, *host.Host, *host.Host) error {
	if c.failCreate {
		return errors.New("failed to create container")
	}
	return nil
}

func (c *dockerClientMock) GetContainer(context.Context, *host.Host, string) (*types.ContainerJSON, error) {
	if c.failGet {
		return nil, errors.New("failed to inspect container")
	}

	container := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    c.generateContainerID(),
			State: &types.ContainerState{Running: true},
		},
		Config: &container.Config{
			ExposedPorts: nat.PortSet{"22/tcp": {}},
		},
		NetworkSettings: &types.NetworkSettings{
			NetworkSettingsBase: types.NetworkSettingsBase{
				Ports: nat.PortMap{
					"22/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "5000",
						},
					},
				},
			},
		},
	}

	if !c.hasOpenPorts {
		container.NetworkSettings = &types.NetworkSettings{}
	}

	return container, nil
}

func (c *dockerClientMock) ListContainers(context.Context, *host.Host) ([]types.Container, error) {
	if c.failList {
		return nil, errors.New("failed to list containers")
	}
	container := types.Container{
		ID: "container-1",
		Ports: []types.Port{
			{PublicPort: 5000},
			{PublicPort: 5001},
		},
		Names: []string{
			"/container-1",
		},
	}
	return []types.Container{container}, nil
}

func (c *dockerClientMock) RemoveContainer(context.Context, *host.Host, string) error {
	if c.failRemove {
		return errors.New("failed to remove container")
	}
	return nil
}

func (c *dockerClientMock) ListImages(context.Context, *host.Host) ([]types.ImageSummary, error) {
	if c.failList {
		return nil, errors.New("failed to list images")
	}
	now := time.Now()
	image1 := types.ImageSummary{
		ID:         "image-1",
		Containers: 2,
		Created:    now.Unix(),
	}
	image2 := types.ImageSummary{
		ID:         "image-2",
		Containers: 2,
		Created:    now.Add(-10 * time.Minute).Unix(),
	}
	return []types.ImageSummary{image1, image2}, nil
}

func (c *dockerClientMock) RemoveImage(context.Context, *host.Host, string) error {
	if c.failRemove {
		return errors.New("failed to remove image")
	}
	return nil
}

func (c *dockerClientMock) StartContainer(context.Context, *host.Host, string) error {
	if c.failStart {
		return errors.New("failed to start container")
	}
	return nil
}

func (c *dockerClientMock) AttachToContainer(context.Context, *host.Host, string, host.DockerOptions) (*types.HijackedResponse, error) {
	if c.failAttach {
		return c.containerAttachment, errors.New("failed to attach to container")
	}
	return c.containerAttachment, nil
}
