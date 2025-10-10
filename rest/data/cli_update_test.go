package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/stretchr/testify/suite"
)

type cliUpdateConnectorSuite struct {
	suite.Suite
	setup   func()
	degrade func()
}

func TestUpdateConnector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &cliUpdateConnectorSuite{}
	s.setup = func() {
		s.NoError(db.ClearCollections(evergreen.ConfigCollection))
	}
	s.degrade = func() {
		flags := evergreen.ServiceFlags{
			CLIUpdatesDisabled: true,
		}
		s.NoError(evergreen.SetServiceFlags(ctx, flags))
	}
	suite.Run(t, s)
}

func (s *cliUpdateConnectorSuite) SetupSuite() {
	s.setup()
}

func (s *cliUpdateConnectorSuite) Test() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	latestRevision := "abcdef"
	env := &mock.Environment{
		EvergreenSettings: &evergreen.Settings{
			AuthConfig: evergreen.AuthConfig{
				OAuth: &evergreen.OAuthConfig{
					Issuer:      "https://example.com",
					ClientID:    "client_id",
					ConnectorID: "connector_id",
				},
			},
		},
	}
	env.Clients.LatestRevision = latestRevision

	v, err := GetCLIUpdate(ctx, env)
	s.Require().NoError(err)
	s.Require().NotNil(v)
	s.Require().NotNil(v.ClientConfig.LatestRevision)
	s.Equal(latestRevision, *v.ClientConfig.LatestRevision)
	s.Equal("https://example.com", *v.ClientConfig.OAuthIssuer)
	s.Equal("client_id", *v.ClientConfig.OAuthClientID)
	s.Equal("connector_id", *v.ClientConfig.OAuthConnectorID)
}

func (s *cliUpdateConnectorSuite) TestDegradedMode() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.degrade()

	latestRevision := "abcdef"
	env := &mock.Environment{}
	env.Clients.LatestRevision = latestRevision

	v, err := GetCLIUpdate(ctx, env)
	s.NoError(err)
	s.Require().NotNil(v)
	s.True(v.IgnoreUpdate)
	s.Require().NotNil(v.ClientConfig.LatestRevision)
	s.Equal(latestRevision, *v.ClientConfig.LatestRevision)
}
