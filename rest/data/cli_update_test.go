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
			Api: evergreen.APIConfig{
				CorpURL: "https://evergreen.corp.example.com",
			},
			AuthConfig: evergreen.AuthConfig{
				OAuth: &evergreen.OAuthConfig{
					Issuer:      "https://example.com",
					ClientID:    "client_id",
					ConnectorID: "connector_id",
				},
			},
			Ui: evergreen.UIConfig{
				UIv2Url: "https://spruce.example.com",
			},
		},
	}
	env.Clients.LatestRevision = latestRevision

	v, err := GetCLIUpdate(ctx, env)
	s.Require().NoError(err)
	s.Require().NotNil(v)
	s.Require().NotNil(v.ClientConfig.LatestRevision)
	s.Equal(latestRevision, *v.ClientConfig.LatestRevision)
	s.Require().NotNil(v.ClientConfig.OAuthIssuer)
	s.Equal("https://example.com", *v.ClientConfig.OAuthIssuer)
	s.Require().NotNil(v.ClientConfig.OAuthClientID)
	s.Equal("client_id", *v.ClientConfig.OAuthClientID)
	s.Require().NotNil(v.ClientConfig.OAuthConnectorID)
	s.Equal("connector_id", *v.ClientConfig.OAuthConnectorID)
	s.Require().NotNil(v.ClientConfig.CorpAPIServerHost)
	s.Equal("https://evergreen.corp.example.com/api", *v.ClientConfig.CorpAPIServerHost)
	s.Require().NotNil(v.ClientConfig.NewUIServerHost)
	s.Equal("https://spruce.example.com", *v.ClientConfig.NewUIServerHost)
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
