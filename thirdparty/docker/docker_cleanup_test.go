package docker

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanup(t *testing.T) {
	dockerClient, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	ctx := context.Background()

	// check if the Docker daemon is running
	_, err = dockerClient.Ping(ctx)
	if err != nil {
		// the daemon isn't running. Make sure Cleanup noops
		assert.NoError(t, Cleanup(context.Background(), grip.NewJournaler("")))
		return
	}

	const imageName = "public.ecr.aws/docker/library/hello-world:latest"
	for name, test := range map[string]func(*testing.T){
		"cleanContainers": func(*testing.T) {
			var resp container.CreateResponse
			resp, err = dockerClient.ContainerCreate(ctx, &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))
			var info types.Info
			info, err = dockerClient.Info(ctx)
			require.NoError(t, err)
			require.Positive(t, info.ContainersRunning)

			assert.NoError(t, cleanContainers(context.Background(), dockerClient))

			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Zero(t, info.Containers)
		},
		"cleanImages": func(*testing.T) {
			assert.NoError(t, cleanImages(context.Background(), dockerClient))

			var info types.Info
			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Zero(t, info.Images)
		},
		"cleanVolumes": func(*testing.T) {
			_, err = dockerClient.VolumeCreate(ctx, volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, volumes.Volumes)

			assert.NoError(t, cleanVolumes(context.Background(), dockerClient, grip.NewJournaler("")))

			volumes, err = dockerClient.VolumeList(ctx, volume.ListOptions{})
			assert.NoError(t, err)
			assert.Empty(t, volumes.Volumes)
		},
		"Cleanup": func(*testing.T) {
			resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))
			info, err := dockerClient.Info(ctx)
			require.NoError(t, err)
			require.Positive(t, info.ContainersRunning)
			require.Positive(t, info.Images)

			_, err = dockerClient.VolumeCreate(ctx, volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, volumes.Volumes)

			assert.NoError(t, Cleanup(context.Background(), grip.NewJournaler("")))

			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Zero(t, info.Containers)
			assert.Zero(t, info.Images)

			volumes, err = dockerClient.VolumeList(ctx, volume.ListOptions{})
			assert.NoError(t, err)
			assert.Empty(t, volumes.Volumes)
		},
	} {
		// Retry pulling the Docker image to work around rate limits on
		// unauthenciated pulls.
		require.NoError(t, utility.Retry(ctx, func() (bool, error) {
			out, err := dockerClient.ImagePull(ctx, imageName, types.ImagePullOptions{})
			if err != nil {
				return true, err
			}
			if _, err := io.Copy(io.Discard, out); err != nil {
				return true, err
			}
			if err := out.Close(); err != nil {
				return true, err
			}
			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: 10,
			MinDelay:    time.Second,
			MaxDelay:    10 * time.Minute,
		}))

		images, err := dockerClient.ImageList(ctx, types.ImageListOptions{All: true})
		require.NoError(t, err)
		require.Len(t, images, 1)

		t.Run(name, test)
	}
}
