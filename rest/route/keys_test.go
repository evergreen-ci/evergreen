package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UserConnectorSuite struct {
	sc   *data.MockConnector
	get  gimlet.RouteHandler
	post gimlet.RouteHandler
	suite.Suite
}

func TestUserConnectorSuite(t *testing.T) {
	s := new(UserConnectorSuite)
	suite.Run(t, s)
}

func (s *UserConnectorSuite) SetupTest() {
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
	s.post = makeSetKey(s.sc)
	s.get = makeFetchKeys(s.sc)
}

func (s *UserConnectorSuite) TestGetSshKeysWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.get.Run(context.TODO())
	})
}

func (s *UserConnectorSuite) TestGetSshKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := s.get.Run(ctx)

	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})
	s.Len(payload, 2)
	for i, result := range payload {
		s.IsType(new(model.APIPubKey), result)
		key := result.(*model.APIPubKey)
		s.Equal(key.Name, model.ToAPIString(fmt.Sprintf("user0_pubkey%d", i)))
	}
}

func (s *UserConnectorSuite) TestGetSshKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	resp := s.get.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
}

func (s *UserConnectorSuite) TestAddSshKeyWithNoUserPanics() {
	s.post.(*keysPostHandler).keyName = "Test"
	s.post.(*keysPostHandler).keyValue = "ssh-rsa 12345"

	s.PanicsWithValue("no user attached to request", func() {
		_ = s.get.Run(context.TODO())
	})
}

func (s *UserConnectorSuite) TestAddSshKey() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	s.post.(*keysPostHandler).keyName = "Test"
	s.post.(*keysPostHandler).keyValue = "ssh-dss 12345"
	resp := s.post.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)
	s.Equal("Test", s.sc.MockUserConnector.CachedUsers["user0"].PubKeys[2].Name)
	s.Equal("ssh-dss 12345", s.sc.MockUserConnector.CachedUsers["user0"].PubKeys[2].Key)
}

func (s *UserConnectorSuite) TestAddDuplicateSshKeyFails() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	s.TestAddSshKey()

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)

	s.post.(*keysPostHandler).keyName = "Test"
	s.post.(*keysPostHandler).keyValue = "ssh-dss 12345"

	resp := s.post.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)
}

func TestKeyValidationFailsWithInvalidKeys(t *testing.T) {
	assert := assert.New(t)

	err := validateKeyName("    ")
	assert.Error(err)
	assert.Equal("empty key name", err.Error())

	err2 := validateKeyValue("    ")
	assert.Error(err2)
	assert.Equal("invalid public key", err2.Error())

	err3 := validateKeyValue("ssh-rsa notvalidbase64")
	assert.Error(err3)
	assert.Equal("invalid public key: key contents invalid", err3.Error())
}

func TestKeyValidation(t *testing.T) {
	err := validateKeyName("key1 ")
	assert.NoError(t, err)

	err = validateKeyValue("ssh-rsa YWJjZDEyMzQK")
	assert.NoError(t, err)
}

type UserConnectorDeleteSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler
	suite.Suite
}

func (s *UserConnectorDeleteSuite) SetupTest() {
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

	s.rm = makeDeleteKeys(s.sc)
}

func (s *UserConnectorDeleteSuite) TestDeleteSshKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	s.rm.(*keysDeleteHandler).keyName = "user0_pubkey0"
	resp := s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 1)

	s.rm.(*keysDeleteHandler).keyName = "user0_pubkey1"
	resp = s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Empty(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys)
}

func (s *UserConnectorDeleteSuite) TestDeleteSshKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	s.rm.(*keysDeleteHandler).keyName = "keythatdoesntexist"
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *UserConnectorDeleteSuite) TestDeleteSshKeysWithNoUserFails() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())
	})
}

func TestUserConnectorDeleteSuite(t *testing.T) {
	s := new(UserConnectorDeleteSuite)
	suite.Run(t, s)
}
