package docker

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/mongodb/grip"
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

	logger.Infof("Removing: %d containers (%d running, %d paused, %d stopped), %d images",
		info.Containers,
		info.ContainersRunning,
		info.ContainersPaused,
		info.ContainersStopped,
		info.Images,
	)

	catcher := grip.NewBasicCatcher()

	// clean up containers
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		catcher.Add(errors.Wrap(err, "can't get list of containers"))
	}
	for _, container := range containers {
		catcher.Add(errors.Wrapf(dockerClient.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true}), "can't remove container '%s'", container.ID))
	}

	// clean up images
	images, err := dockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		catcher.Add(errors.Wrap(err, "can't get image list"))
	}
	for _, image := range images {
		resp, err := dockerClient.ImageRemove(ctx, image.ID, types.ImageRemoveOptions{Force: true})
		if err != nil {
			catcher.Add(errors.Wrapf(err, "can't remove image '%s'", image.ID))
		}
		if len(resp) != 1 || resp[0].Deleted != image.ID {
			catcher.Add(errors.Wrapf(err, "image '%s' wasn't deleted", image.ID))
		}
	}

	// clean up volumes
	volumes, err := dockerClient.VolumeList(ctx, filters.Args{})
	if err != nil {
		catcher.Add(errors.Wrap(err, "can't get volume list"))
	}
	logger.Infof("Removing %d volumes", len(volumes.Volumes))
	for _, volume := range volumes.Volumes {
		catcher.Add(errors.Wrapf(dockerClient.VolumeRemove(ctx, volume.Name, true), "can't remove volume '%s'", volume.Name))
	}

	return catcher.Resolve()
}
