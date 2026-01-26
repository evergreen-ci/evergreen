package docker

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Cleanup removes all containers, images, and volumes from a Docker instance
// if a Docker daemon is listening on the default socket
func Cleanup(ctx context.Context, logger grip.Journaler) error {
	dockerClient, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		return errors.Wrap(err, "can't get Docker client")
	}

	info, err := dockerClient.Info(ctx)
	if err != nil {
		logger.Info("Can't connect to Docker. It's probably not running")
		return nil
	}

	logger.Info(message.Fields{
		"message":            "removing docker artifacts",
		"containers_total":   info.Containers,
		"containers_running": info.ContainersRunning,
		"containers_paused":  info.ContainersPaused,
		"containers_stopped": info.ContainersStopped,
		"images":             info.Images,
	})

	catcher := grip.NewBasicCatcher()
	catcher.Add(cleanContainers(ctx, dockerClient, logger))
	catcher.Add(cleanImages(ctx, dockerClient, logger))
	catcher.Add(cleanVolumes(ctx, dockerClient, logger))
	catcher.Add(cleanNetworks(ctx, dockerClient, logger))

	return catcher.Resolve()
}

func cleanContainers(ctx context.Context, dockerClient *client.Client, logger grip.Journaler) error {
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return errors.Wrap(err, "getting containers list")
	}

	logger.Infof("Removing %d containers", len(containers))
	catcher := grip.NewBasicCatcher()
	for _, c := range containers {
		catcher.Wrapf(dockerClient.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}), "removing container '%s'", c.ID)
	}

	return catcher.Resolve()
}

func cleanImages(ctx context.Context, dockerClient *client.Client, logger grip.Journaler) error {
	images, err := dockerClient.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return errors.Wrap(err, "getting image list")
	}

	logger.Infof("Removing %d images", len(images))
	catcher := grip.NewBasicCatcher()
	for _, img := range images {
		_, err := dockerClient.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: true})
		catcher.Wrapf(err, "removing image '%s'", img.ID)
	}

	return catcher.Resolve()
}

func cleanVolumes(ctx context.Context, dockerClient *client.Client, logger grip.Journaler) error {
	volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "getting volume list")
	}

	logger.Infof("Removing %d volumes", len(volumes.Volumes))
	catcher := grip.NewBasicCatcher()
	for _, volume := range volumes.Volumes {
		catcher.Wrapf(dockerClient.VolumeRemove(ctx, volume.Name, true), "removing volume '%s'", volume.Name)
	}

	return catcher.Resolve()
}

func cleanNetworks(ctx context.Context, dockerClient *client.Client, logger grip.Journaler) error {
	networks, err := dockerClient.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			// Filter out built-in networks. They come with Docker by default
			// and cannot be removed.
			Key:   "type",
			Value: "custom",
		}),
	})
	if err != nil {
		return errors.Wrap(err, "getting network list")
	}

	logger.Infof("Removing %d networks", len(networks))
	catcher := grip.NewBasicCatcher()
	for _, network := range networks {
		catcher.Wrapf(dockerClient.NetworkRemove(ctx, network.ID), "removing network '%s'", network.ID)
	}

	return catcher.Resolve()
}
