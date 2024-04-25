package docker

import (
	"context"
	"io"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
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
			require.True(t, info.ContainersRunning > 0)

			assert.NoError(t, cleanContainers(context.Background(), dockerClient))

			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Equal(t, 0, info.Containers)
		},
		"cleanImages": func(*testing.T) {
			assert.NoError(t, cleanImages(context.Background(), dockerClient))

			var info types.Info
			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Equal(t, 0, info.Images)
		},
		"cleanVolumes": func(*testing.T) {
			_, err = dockerClient.VolumeCreate(ctx, volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
			require.NoError(t, err)
			require.True(t, len(volumes.Volumes) > 0)

			assert.NoError(t, cleanVolumes(context.Background(), dockerClient, grip.NewJournaler("")))

			volumes, err = dockerClient.VolumeList(ctx, volume.ListOptions{})
			assert.NoError(t, err)
			assert.Len(t, volumes.Volumes, 0)
		},
		"Cleanup": func(*testing.T) {
			resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))
			info, err := dockerClient.Info(ctx)
			require.NoError(t, err)
			require.True(t, info.ContainersRunning > 0)
			require.True(t, info.Images > 0)

			_, err = dockerClient.VolumeCreate(ctx, volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
			require.NoError(t, err)
			require.True(t, len(volumes.Volumes) > 0)

			assert.NoError(t, Cleanup(context.Background(), grip.NewJournaler("")))

			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Equal(t, 0, info.Containers)
			assert.Equal(t, 0, info.Images)

			volumes, err = dockerClient.VolumeList(ctx, volume.ListOptions{})
			assert.NoError(t, err)
			assert.Len(t, volumes.Volumes, 0)
		},
	} {
		out, err := dockerClient.ImagePull(ctx, imageName, types.ImagePullOptions{})
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, out)
		require.NoError(t, err)
		require.NoError(t, out.Close())

		info, err := dockerClient.Info(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, info.Images)

		t.Run(name, test)
	}
}
