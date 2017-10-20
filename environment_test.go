package evergreen

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type EnvironmentSuite struct {
	path string
	env  *envState
	suite.Suite
}

func TestEnvironmentSuite(t *testing.T) {
	assert.Implements(t, (*Environment)(nil), &envState{})

	suite.Run(t, new(EnvironmentSuite))
}

func (s *EnvironmentSuite) SetupSuite() {
	s.path = os.Getenv("SETTINGS_OVERRIDE")
}

func (s *EnvironmentSuite) shouldSkip() {
	if s.path == "" {
		s.T().Skip("settings not configured")
	}
}

func (s *EnvironmentSuite) SetupTest() {
	s.env = &envState{}
}

func (s *EnvironmentSuite) TestLoadingConfig() {
	s.shouldSkip()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.env.Configure(ctx, s.path))
	s.Error(s.env.Configure(ctx, s.path))
}

func (s *EnvironmentSuite) TestLoadingConfigErrorsIfFileDoesNotExist() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Error(s.env.Configure(ctx, ""))

	s.Error(s.env.Configure(ctx, s.path+"-DOES-NOT-EXIST"))
}

func (s *EnvironmentSuite) TestConfigErrorsIfCannotValidateConfig() {
	s.env.settings = &Settings{}
	err := s.env.initSettings("")
	s.Error(err)
	s.Contains(err.Error(), "validating settings")
}
