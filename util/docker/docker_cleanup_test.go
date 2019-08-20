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
	out, err := dockerClient.ImagePull(ctx, "hello-world", types.ImagePullOptions{})
	require.NoError(t, err)
	defer out.Close()
	io.Copy(ioutil.Discard, out)

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "hello-world",
	}, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))

	_, err = dockerClient.VolumeCreate(ctx, volume.VolumeCreateBody{})
	require.NoError(t, err)

	info, err := dockerClient.Info(ctx)
	require.NoError(t, err)
	require.True(t, info.Containers > 0)
	require.True(t, info.Images > 0)

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
}
