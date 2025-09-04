package docker

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanup(t *testing.T) {
	dockerClient, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	// check if the Docker daemon is running
	_, err = dockerClient.Ping(t.Context())
	if err != nil {
		// the daemon isn't running. Make sure Cleanup noops
		assert.NoError(t, Cleanup(t.Context(), grip.NewJournaler("")))
		return
	}

	const imageName = "public.ecr.aws/docker/library/hello-world:latest"
	for name, test := range map[string]func(*testing.T){
		"cleanContainers": func(t *testing.T) {
			var resp container.CreateResponse
			resp, err = dockerClient.ContainerCreate(t.Context(), &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(t.Context(), resp.ID, types.ContainerStartOptions{}))
			var info types.Info
			info, err = dockerClient.Info(t.Context())
			require.NoError(t, err)
			require.Positive(t, info.ContainersRunning)

			assert.NoError(t, cleanContainers(t.Context(), dockerClient))

			info, err = dockerClient.Info(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, info.Containers)
		},
		"cleanImages": func(t *testing.T) {
			assert.NoError(t, cleanImages(t.Context(), dockerClient))

			var info types.Info
			info, err = dockerClient.Info(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, info.Images)
		},
		"cleanVolumes": func(t *testing.T) {
			_, err = dockerClient.VolumeCreate(t.Context(), volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(t.Context(), volume.ListOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, volumes.Volumes)

			assert.NoError(t, cleanVolumes(t.Context(), dockerClient, grip.NewJournaler("")))

			volumes, err = dockerClient.VolumeList(t.Context(), volume.ListOptions{})
			assert.NoError(t, err)
			assert.Empty(t, volumes.Volumes)
		},
		"Cleanup": func(t *testing.T) {
			resp, err := dockerClient.ContainerCreate(t.Context(), &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(t.Context(), resp.ID, types.ContainerStartOptions{}))
			info, err := dockerClient.Info(t.Context())
			require.NoError(t, err)
			require.Positive(t, info.ContainersRunning)
			require.Positive(t, info.Images)

			_, err = dockerClient.VolumeCreate(t.Context(), volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(t.Context(), volume.ListOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, volumes.Volumes)

			assert.NoError(t, Cleanup(t.Context(), grip.NewJournaler("")))

			info, err = dockerClient.Info(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, info.Containers)
			assert.Zero(t, info.Images)

			volumes, err = dockerClient.VolumeList(t.Context(), volume.ListOptions{})
			assert.NoError(t, err)
			assert.Empty(t, volumes.Volumes)
		},
	} {
		// Retry pulling the Docker image to work around rate limits on
		// unauthenciated pulls.
		require.NoError(t, utility.Retry(t.Context(), func() (bool, error) {
			out, err := dockerClient.ImagePull(t.Context(), imageName, types.ImagePullOptions{})
			if err != nil {
				return true, err
			}
			b, err := io.ReadAll(out)
			if err != nil {
				return true, err
			}
			if strings.Contains(string(b), "Rate exceeded") {
				// The image pull can return no error and also no image if the
				// rate limit is exceeded.
				return true, errors.Errorf("rate limit exceeded pulling image '%s'", imageName)
			}
			if err := out.Close(); err != nil {
				return true, err
			}
			images, err := dockerClient.ImageList(t.Context(), types.ImageListOptions{All: true})
			if err != nil {
				return true, err
			}
			if len(images) == 0 {
				return true, errors.Errorf("no images found after pulling image '%s'", imageName)
			}

			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: 10,
			MinDelay:    time.Second,
			MaxDelay:    10 * time.Minute,
		}))

		t.Run(name, test)
	}
}
