package thirdparty

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

func TestRuntimeEnvironmentsSuite(t *testing.T) {
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestRuntimeEnvironmentsSuite")

	suite.Run(t, &runtimeEnvironmentsSuite{config: config})
}

type runtimeEnvironmentsSuite struct {
	suite.Suite
	config *evergreen.Settings

	ctx    context.Context
	cancel func()
}

func (s *runtimeEnvironmentsSuite) SetupTest() {
	var err error
	s.NoError(err)

	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)
}

func (s *runtimeEnvironmentsSuite) TearDownTest() {
	s.NoError(s.ctx.Err())
	s.cancel()
}

// TODO: Uncomment when DEVPROD-6983 is resolved. Right now, the API does not work on task hosts.
// func (s *runtimeEnvironmentsSuite) TestGetImageNames() {
// 	result, err := getImageNames(s.ctx, s.config.RuntimeEnvironments.BaseURL, s.config.RuntimeEnvironments.APIKey)
// 	s.NoError(err)
// 	s.NotEmpty(result)
// 	s.NotContains(result, "")
// }
