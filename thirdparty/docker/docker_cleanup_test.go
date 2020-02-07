package docker

import (
	"context"
	"io"
	"io/ioutil"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanup(t *testing.T) {
	dockerClient, err := client.NewClientWithOpts()
	require.NoError(t, err)
	ctx := context.Background()

	// check if the Docker daemon is running
	_, err = dockerClient.Ping(ctx)
	if err != nil {
		// the daemon isn't running. Make sure Cleanup noops
		assert.NoError(t, Cleanup(context.Background(), grip.NewJournaler("")))
		return
	}

	for name, test := range map[string]func(*testing.T){
		"cleanContainers": func(*testing.T) {
			var resp container.ContainerCreateCreatedBody
			resp, err = dockerClient.ContainerCreate(ctx, &container.Config{
				Image: "hello-world",
			}, nil, nil, "")
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
			_, err = dockerClient.VolumeCreate(ctx, volume.VolumeCreateBody{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(ctx, filters.Args{})
			require.NoError(t, err)
			require.True(t, len(volumes.Volumes) > 0)

			assert.NoError(t, cleanVolumes(context.Background(), dockerClient, grip.NewJournaler("")))

			volumes, err = dockerClient.VolumeList(ctx, filters.Args{})
			assert.NoError(t, err)
			assert.Len(t, volumes.Volumes, 0)
		},
		"Cleanup": func(*testing.T) {
			resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
				Image: "hello-world",
			}, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))
			info, err := dockerClient.Info(ctx)
			require.NoError(t, err)
			require.True(t, info.ContainersRunning > 0)
			require.True(t, info.Images > 0)

			_, err = dockerClient.VolumeCreate(ctx, volume.VolumeCreateBody{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(ctx, filters.Args{})
			require.NoError(t, err)
			require.True(t, len(volumes.Volumes) > 0)

			assert.NoError(t, Cleanup(context.Background(), grip.NewJournaler("")))

			info, err = dockerClient.Info(ctx)
			assert.NoError(t, err)
			assert.Equal(t, 0, info.Containers)
			assert.Equal(t, 0, info.Images)

			volumes, err = dockerClient.VolumeList(ctx, filters.Args{})
			assert.NoError(t, err)
			assert.Len(t, volumes.Volumes, 0)
		},
	} {
		out, err := dockerClient.ImagePull(ctx, "hello-world", types.ImagePullOptions{})
		require.NoError(t, err)
		_, err = io.Copy(ioutil.Discard, out)
		require.NoError(t, err)
		require.NoError(t, out.Close())

		info, err := dockerClient.Info(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, info.Images)

		t.Run(name, test)
	}
}
