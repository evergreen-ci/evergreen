package docker

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Cleanup removes all containers, images, and volumes from a Docker instance
// if a Docker daemon is listening on the default socket
func Cleanup(ctx context.Context, logger grip.Journaler) error {
	dockerClient, err := client.NewClientWithOpts()
	if err != nil {
		return errors.Wrap(err, "can't get Docker client")
	}

	info, err := dockerClient.Info(ctx)
	if err != nil {
		if client.IsErrConnectionFailed(err) {
			// the daemon isn't running so there's nothing to clean up
			logger.Info("Docker daemon isn't running")
			return nil
		}
		return errors.Wrap(err, "can't get Docker info")
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
	catcher.Add(cleanContainers(ctx, dockerClient))
	catcher.Add(cleanImages(ctx, dockerClient))
	catcher.Add(cleanVolumes(ctx, dockerClient, logger))

	return catcher.Resolve()
}

func cleanContainers(ctx context.Context, dockerClient *client.Client) error {
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return errors.Wrap(err, "can't get list of containers")
	}

	catcher := grip.NewBasicCatcher()
	for _, container := range containers {
		catcher.Wrapf(dockerClient.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true}), "can't remove container '%s'", container.ID)
	}

	return catcher.Resolve()
}

func cleanImages(ctx context.Context, dockerClient *client.Client) error {
	images, err := dockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		return errors.Wrap(err, "can't get image list")
	}

	catcher := grip.NewBasicCatcher()
	for _, image := range images {
		resp, err := dockerClient.ImageRemove(ctx, image.ID, types.ImageRemoveOptions{Force: true})
		catcher.Wrapf(err, "can't remove image '%s'", image.ID)
		if len(resp) != 1 || resp[0].Deleted != image.ID {
			catcher.Errorf("image '%s' wasn't deleted", image.ID)
		}
	}

	return catcher.Resolve()
}

func cleanVolumes(ctx context.Context, dockerClient *client.Client, logger grip.Journaler) error {
	volumes, err := dockerClient.VolumeList(ctx, filters.Args{})
	if err != nil {
		return errors.Wrap(err, "can't get volume list")
	}

	logger.Infof("Removing %d volumes", len(volumes.Volumes))
	catcher := grip.NewBasicCatcher()
	for _, volume := range volumes.Volumes {
		catcher.Wrapf(dockerClient.VolumeRemove(ctx, volume.Name, true), "can't remove volume '%s'", volume.Name)
	}

	return catcher.Resolve()
}
