package cloud

import (
	"os"
	"testing"
	"time"

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
	require.NoError(t, err)

	// Verify that the Docker client can reach the Docker daemon before unit
	// tests.
	err = utility.Retry(t.Context(), func() (bool, error) {
		_, err = dockerClient.Ping(t.Context())
		return err != nil, err
	}, utility.RetryOptions{
		MaxAttempts: 5,
		MinDelay:    time.Second,
	})
	require.NoError(t, err)

	suite.Run(t, s)
}
