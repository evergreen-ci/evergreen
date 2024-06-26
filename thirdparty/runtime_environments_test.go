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

func (s *runtimeEnvironmentsSuite) TestGetImageList() {
	result, err := getImageNames(s.ctx, s.config.runtime_environments.base_url, s.config.runtime_environments.api_key)
	s.NoError(err)
	s.NotEmpty(result)
}
