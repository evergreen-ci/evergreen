package docker

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
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
		"CleanContainers": func(t *testing.T) {
			var resp container.CreateResponse
			resp, err = dockerClient.ContainerCreate(t.Context(), &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(t.Context(), resp.ID, container.StartOptions{}))
			var info system.Info
			info, err = dockerClient.Info(t.Context())
			require.NoError(t, err)
			require.Positive(t, info.ContainersRunning)

			assert.NoError(t, cleanContainers(t.Context(), dockerClient, grip.NewJournaler("")))

			info, err = dockerClient.Info(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, info.Containers)
		},
		"CleanImages": func(t *testing.T) {
			assert.NoError(t, cleanImages(t.Context(), dockerClient, grip.NewJournaler("")))

			var info system.Info
			info, err = dockerClient.Info(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, info.Images)
		},
		"CleanVolumes": func(t *testing.T) {
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
		"CleanNetworks": func(t *testing.T) {
			_, err := dockerClient.NetworkCreate(t.Context(), "test-network", network.CreateOptions{})
			require.NoError(t, err)

			customNetworkFilter := filters.NewArgs(filters.KeyValuePair{
				Key:   "type",
				Value: "custom",
			})
			networks, err := dockerClient.NetworkList(t.Context(), network.ListOptions{
				Filters: customNetworkFilter,
			})
			require.NoError(t, err)
			require.NotEmpty(t, networks)

			assert.NoError(t, cleanNetworks(t.Context(), dockerClient, grip.NewJournaler("")))

			networks, err = dockerClient.NetworkList(t.Context(), network.ListOptions{
				Filters: customNetworkFilter,
			})
			assert.NoError(t, err)
			assert.Empty(t, networks)
		},
		"Cleanup": func(t *testing.T) {
			resp, err := dockerClient.ContainerCreate(t.Context(), &container.Config{
				Image: imageName,
			}, nil, nil, nil, "")
			require.NoError(t, err)
			require.NoError(t, dockerClient.ContainerStart(t.Context(), resp.ID, container.StartOptions{}))
			info, err := dockerClient.Info(t.Context())
			require.NoError(t, err)
			require.Positive(t, info.ContainersRunning)
			require.Positive(t, info.Images)

			_, err = dockerClient.VolumeCreate(t.Context(), volume.CreateOptions{})
			require.NoError(t, err)
			volumes, err := dockerClient.VolumeList(t.Context(), volume.ListOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, volumes.Volumes)

			_, err = dockerClient.NetworkCreate(t.Context(), "test-network", network.CreateOptions{})
			require.NoError(t, err)
			customNetworkFilter := filters.NewArgs(filters.KeyValuePair{
				Key:   "type",
				Value: "custom",
			})
			networks, err := dockerClient.NetworkList(t.Context(), network.ListOptions{
				Filters: customNetworkFilter,
			})
			require.NoError(t, err)
			require.NotEmpty(t, networks)

			assert.NoError(t, Cleanup(t.Context(), grip.NewJournaler("")))

			info, err = dockerClient.Info(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, info.Containers)
			assert.Zero(t, info.Images)

			volumes, err = dockerClient.VolumeList(t.Context(), volume.ListOptions{})
			assert.NoError(t, err)
			assert.Empty(t, volumes.Volumes)

			networks, err = dockerClient.NetworkList(t.Context(), network.ListOptions{
				Filters: customNetworkFilter,
			})
			assert.NoError(t, err)
			assert.Empty(t, networks)
		},
	} {
		// Retry pulling the Docker image to work around rate limits on
		// unauthenciated pulls.
		require.NoError(t, utility.Retry(t.Context(), func() (bool, error) {
			out, err := dockerClient.ImagePull(t.Context(), imageName, image.PullOptions{})
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
			images, err := dockerClient.ImageList(t.Context(), image.ListOptions{All: true})
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
			MaxDelay:    30 * time.Second,
		}))

		t.Run(name, test)
	}
}
