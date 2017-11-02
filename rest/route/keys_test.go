package route

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UserConnectorSuite struct {
	sc *data.MockConnector
	rm *RouteManager
	suite.Suite
}

func TestUserConnectorSuite(t *testing.T) {
	s := new(UserConnectorSuite)
	suite.Run(t, s)
}

func (s *UserConnectorSuite) SetupTest() {
	s.rm = getKeysRouteManager("", 2)
	s.sc = &data.MockConnector{MockUserConnector: data.MockUserConnector{
		CachedUsers: map[string]*user.DBUser{
			"user0": {
				Id:     "user0",
				APIKey: "apikey0",
				PubKeys: []user.PubKey{
					{
						Name:      "user0_pubkey0",
						Key:       "ssh-mock 12345",
						CreatedAt: time.Now(),
					},
					{
						Name:      "user0_pubkey1",
						Key:       "ssh-mock 67890",
						CreatedAt: time.Now(),
					},
				},
			},
			"user1": {
				Id:     "user1",
				APIKey: "apikey1",
				// no pub keys
			},
		},
	}}
}

func (s *UserConnectorSuite) TestGetSshKeysWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Methods[0].Execute(context.TODO(), s.sc)
	})
}

func (s *UserConnectorSuite) TestGetSshKeys() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	data, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.Len(data.Result, 2)
	s.NoError(err)
	for i, result := range data.Result {
		s.IsType(new(model.APIPubKey), result)
		key := result.(*model.APIPubKey)
		s.Equal(key.Name, model.APIString(fmt.Sprintf("user0_pubkey%d", i)))
	}
}

func (s *UserConnectorSuite) TestGetSshKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user1"])

	data, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.Len(data.Result, 0)
	s.NoError(err)
}

func (s *UserConnectorSuite) TestAddSshKeyWithNoUserPanics() {
	s.rm.Methods[1].RequestHandler.(*keysPostHandler).keyName = "Test"
	s.rm.Methods[1].RequestHandler.(*keysPostHandler).keyValue = "ssh-rsa 12345"

	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Methods[1].Execute(context.TODO(), s.sc)
	})
}

func (s *UserConnectorSuite) TestAddSshKey() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])

	s.rm.Methods[1].RequestHandler.(*keysPostHandler).keyName = "Test"
	s.rm.Methods[1].RequestHandler.(*keysPostHandler).keyValue = "ssh-dss 12345"
	data, err := s.rm.Methods[1].Execute(ctx, s.sc)

	s.Empty(data.Result)
	s.NoError(err)

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)
	s.Equal(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys[2].Name, "Test")
	s.Equal(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys[2].Key, "ssh-dss 12345")
}

func (s *UserConnectorSuite) TestAddDuplicateSshKeyFails() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, s.sc.MockUserConnector.CachedUsers["user0"])
	s.TestAddSshKey()

	data, err := s.rm.Methods[1].Execute(ctx, s.sc)

	s.Empty(data.Result)
	s.Error(err)

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)
}

func TestPostHandlerKeyWithWhiteSpaceValuesFails(t *testing.T) {
	assert := assert.New(t)

	h := keysPostHandler{}
	key := model.APIPubKey{
		Name: "    ",
		Key:  "    ",
	}
	err := h.validatePublicKey(key)
	assert.Error(err)

	assert.Equal(err.Error(), "empty key name")
}
