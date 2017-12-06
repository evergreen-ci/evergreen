// +build go1.7

package cloud

import (
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// sshdPort exposed port (set to 22/tcp, default ssh port)
	sshdPort nat.Port = "22/tcp"
	// address to bind container sshd client to
	bindIP = "localhost"
)

// makeHostConfig generates a host configuration struct that binds a container's SSH port
// to an open port on the host machine. An open port must be in the port range specified
// by the provider settings, but must not a port already being used by existing containers.
// If no ports are available on the host machine, makeHostConfig errors.
func makeHostConfig(d *distro.Distro, s *dockerSettings, containers []types.Container) (*container.HostConfig, error) {
	hostConfig := &container.HostConfig{}
	minPort := s.PortRange.MinPort
	maxPort := s.PortRange.MaxPort

	reservedPorts := make(map[uint16]bool)
	for _, c := range containers {
		for _, p := range c.Ports {
			reservedPorts[p.PublicPort] = true
		}
	}

	hostConfig.PortBindings = make(nat.PortMap)
	for i := minPort; i <= maxPort; i++ {
		// if port is not already in use, bind it to sshd exposed container port
		if !reservedPorts[i] {
			hostConfig.PortBindings[sshdPort] = []nat.PortBinding{
				{
					HostIP:   bindIP,
					HostPort: fmt.Sprintf("%d", i),
				},
			}
			break
		}
	}

	// If map is empty, no ports were available.
	if len(hostConfig.PortBindings) == 0 {
		err := errors.New("No available ports in specified range")
		grip.Error(err)
		return nil, err
	}

	return hostConfig, nil
}

// retrieveOpenPortBinding retrieves a port in the given container that is open to
// SSH access from external connections.
func retrieveOpenPortBinding(containerPtr *types.ContainerJSON) (string, error) {
	exposedPorts := containerPtr.Config.ExposedPorts
	ports := containerPtr.NetworkSettings.Ports

	for k := range exposedPorts {
		portBindings := ports[k]
		if len(portBindings) > 0 {
			return portBindings[0].HostPort, nil
		}
	}
	return "", errors.New("No available ports")
}

// toEvgStatus converts a container state to an Evergreen cloud provider status.
func toEvgStatus(s *types.ContainerState) CloudStatus {
	if s.Running {
		return StatusRunning
	} else if s.Paused {
		return StatusStopped
	} else if s.Restarting {
		return StatusInitializing
	} else if s.OOMKilled || s.Dead {
		return StatusTerminated
	}
	return StatusUnknown
}
