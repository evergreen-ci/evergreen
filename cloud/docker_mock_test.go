// +build go1.7

package cloud

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

type dockerClientMock struct {
	// API call options
	failInit   bool
	failCreate bool
	failGet    bool
	failList   bool
	failRemove bool
	failStart  bool

	// Other options
	hasOpenPorts bool
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

func (c *dockerClientMock) CreateContainer(context.Context, string, *distro.Distro, *dockerSettings) error {
	if c.failCreate {
		return errors.New("failed to create container")
	}
	return nil
}

func (c *dockerClientMock) GetContainer(context.Context, *host.Host) (*types.ContainerJSON, error) {
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
						{"0.0.0.0", "5000"},
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

func (c *dockerClientMock) ListContainers(context.Context, *distro.Distro) ([]types.Container, error) {
	if c.failList {
		return nil, errors.New("failed to list containers")
	}
	container := types.Container{
		Ports: []types.Port{
			{PublicPort: 5000},
			{PublicPort: 5001},
		},
	}
	return []types.Container{container}, nil
}

func (c *dockerClientMock) RemoveContainer(context.Context, *host.Host) error {
	if c.failRemove {
		return errors.New("failed to remove container")
	}
	return nil
}

func (c *dockerClientMock) StartContainer(context.Context, *host.Host) error {
	if c.failStart {
		return errors.New("failed to start container")
	}
	return nil
}
