package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
)

type ServiceFlagsSuite struct {
	suite.Suite
}

func TestServiceFlagsSuite(t *testing.T) {
	suite.Run(t, new(ServiceFlagsSuite))
}

func (s *ServiceFlagsSuite) SetupSuite() {

}

func (s *ServiceFlagsSuite) TearDownTest() {
	s.NoError(db.ClearCollections(evergreen.ConfigCollection))
}

func (s *ServiceFlagsSuite) TestServiceFlagsGet() {
	ctx := context.Background()
	route := makeFetchServiceFlags().(*serviceFlagsGetHandler)

	testSettings := &evergreen.Settings{
		ServiceFlags: evergreen.ServiceFlags{
			StaticAPIKeysDisabled:  true,
			JWTTokenForCLIDisabled: true,
			// this shouldn't be returned
			HostInitDisabled: true,
		},
	}
	s.NoError(evergreen.UpdateConfig(ctx, testSettings))

	resp := route.Run(ctx)
	s.NotNil(resp)
	s.Equal(resp.Status(), 200)

	flags, ok := resp.Data().(evergreen.ServiceFlags)
	s.True(ok)
	s.Require().NotNil(flags)
	s.Equal(testSettings.ServiceFlags.StaticAPIKeysDisabled, flags.StaticAPIKeysDisabled)
	s.Equal(testSettings.ServiceFlags.JWTTokenForCLIDisabled, flags.JWTTokenForCLIDisabled)
	// ensure it only returns the necessary flags
	s.NotEqual(testSettings.ServiceFlags.HostInitDisabled, flags.HostInitDisabled)
}
