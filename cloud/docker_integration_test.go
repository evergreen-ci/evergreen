package cloud

/*
import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
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
	testutil.ConfigureIntegrationTest(t, settings, "TestDockerIntegrationSuite")
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
	assert.NoError(t, s.client.Init("1.37"))
	suite.Run(t, s)
}

func (s *DockerIntegrationSuite) TestImagePull() {
	var err error
	ctx := context.Background()
	err = util.Retry(ctx, func() (bool, error) {
		timeStarted := time.Now()
		err = s.client.pullImage(ctx, &s.host, "docker.io/library/hello-world", "", "")
		if err != nil {
			grip.Debug(message.WrapError(err, message.Fields{
				"message":       "called pullImage",
				"reference":     "EVG-5959",
				"duration_secs": time.Since(timeStarted),
				"host":          s.host.Id,
			}))
			return true, err
		}
		return false, nil
	}, 10, time.Second, 10*time.Minute)

	images, err := s.client.client.ImageList(ctx, types.ImageListOptions{All: true})
	s.NoError(err)
	s.Require().Len(images, 1)
	grip.Info(images)
	s.Contains(images[0].RepoTags, "hello-world:latest")
}
*/
