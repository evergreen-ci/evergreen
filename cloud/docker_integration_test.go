package cloud

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DockerIntegrationSuite struct {
	host     host.Host
	settings *evergreen.Settings
	client   dockerClientImpl
	suite.Suite
}

func TestDockerIntegrationSuite(t *testing.T) {
	dns := os.Getenv("DOCKER_HOST")
	if dns == "" {
		t.Skip()
	}
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings)
	s := &DockerIntegrationSuite{
		host: host.Host{
			Host: dns,
			ContainerPoolSettings: &evergreen.ContainerPool{
				Port: 2376,
			},
		},
		settings: settings,
		client: dockerClientImpl{
			evergreenSettings: settings,
		},
	}
	require.NoError(t, s.client.Init(""))
	dockerClient, err := s.client.generateClient(&s.host)

	// kim: NOTE: ping and info both work and can retrieve info. Using API
	// v1.40 based on ping output.
	ping, err := dockerClient.Ping(t.Context())
	require.NoError(t, err)
	fmt.Printf("Docker server info: %#v\n", ping)
	info, err := dockerClient.Info(t.Context())
	require.NoError(t, err)
	fmt.Printf("Docker info: %#v\n", info)

	suite.Run(t, s)
}

func (s *DockerIntegrationSuite) TestImagePull() {
	const imageName = "public.ecr.aws/docker/library/hello-world:latest"
	var err error
	ctx := s.T().Context()

	// Retry pulling the Docker image to work around rate limits on
	// unauthenciated pulls.
	// kim: NOTE: this doesn't work when upgrading v24 to v28. Unsure why.
	err = utility.Retry(ctx, func() (bool, error) {
		err = s.client.pullImage(ctx, &s.host, imageName, "", "")
		if err != nil {
			return true, err
		}
		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: 10,
		MinDelay:    time.Second,
		MaxDelay:    30 * time.Second,
	})
	s.NoError(err)

	images, err := s.client.client.ImageList(ctx, image.ListOptions{All: true})
	s.NoError(err)
	s.Require().Len(images, 1)
	s.Contains(images[0].RepoTags, imageName)
}
