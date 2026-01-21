package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UserConnectorSuite struct {
	get  gimlet.RouteHandler
	post gimlet.RouteHandler
	suite.Suite
}

func TestUserConnectorSuite(t *testing.T) {
	s := new(UserConnectorSuite)
	suite.Run(t, s)
}

func (s *UserConnectorSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection))
	user0 := user.DBUser{
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
	}
	user1 := user.DBUser{
		Id:     "user1",
		APIKey: "apikey1",
		// no pub keys
	}
	s.NoError(user0.Insert(s.T().Context()))
	s.NoError(user1.Insert(s.T().Context()))
	s.post = makeSetKey()
	s.get = makeFetchKeys()
}

func (s *UserConnectorSuite) TestGetSSHKeysWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.get.Run(context.TODO())
	})
}

func (s *UserConnectorSuite) TestGetSSHKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0", PubKeys: []user.PubKey{{Name: "user0_pubkey0"}, {Name: "user0_pubkey1"}}})

	resp := s.get.Run(ctx)

	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]any)
	s.Len(payload, 2)
	for i, result := range payload {
		s.IsType(new(model.APIPubKey), result)
		key := result.(*model.APIPubKey)
		s.Equal(key.Name, utility.ToStringPtr(fmt.Sprintf("user0_pubkey%d", i)))
	}
}

func (s *UserConnectorSuite) TestGetSSHKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	resp := s.get.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
}

func (s *UserConnectorSuite) TestAddSSHKeyWithNoUserPanics() {
	s.post.(*keysPostHandler).keyName = "Test"
	s.post.(*keysPostHandler).keyValue = "ssh-rsa 12345"

	s.PanicsWithValue("no user attached to request", func() {
		_ = s.get.Run(context.TODO())
	})
}

func (s *UserConnectorSuite) TestAddSSHKey() {
	ctx := context.Background()
	user0, err := user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)
	ctx = gimlet.AttachUser(ctx, user0)

	s.post.(*keysPostHandler).keyName = "Test"
	s.post.(*keysPostHandler).keyValue = "ssh-dss 12345"
	resp := s.post.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())

	user0, err = user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)
	s.Len(user0.PubKeys, 3)
	s.Equal("Test", user0.PubKeys[2].Name)
	s.Equal("ssh-dss 12345", user0.PubKeys[2].Key)
}

func (s *UserConnectorSuite) TestAddDuplicateSSHKeyFails() {
	ctx := context.Background()
	user0, err := user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)

	ctx = gimlet.AttachUser(ctx, user0)
	s.TestAddSSHKey()

	user0, err = user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)
	s.Len(user0.PubKeys, 3)

	s.post.(*keysPostHandler).keyName = "Test"
	s.post.(*keysPostHandler).keyValue = "ssh-dss 12345"

	resp := s.post.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())

	user0, err = user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)
	s.Len(user0.PubKeys, 3)
}

func TestKeyValidationFailsWithInvalidKeys(t *testing.T) {
	assert := assert.New(t)

	err := validateKeyName("    ")
	assert.Error(err)
	assert.Equal("empty key name", err.Error())

	err2 := validateKeyValue("    ")
	assert.Error(err2)
	assert.Contains(err2.Error(), "invalid public key")

	err3 := validateKeyValue("ssh-rsa notvalidbase64")
	assert.Error(err3)
	assert.Equal("public key contents are invalid", err3.Error())
}

func TestKeyValidation(t *testing.T) {
	err := validateKeyName("key1 ")
	assert.NoError(t, err)

	err = validateKeyValue("ssh-rsa YWJjZDEyMzQK")
	assert.NoError(t, err)
}

type UserConnectorDeleteSuite struct {
	rm gimlet.RouteHandler
	suite.Suite
}

func (s *UserConnectorDeleteSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection))
	user0 := user.DBUser{
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
	}
	user1 := user.DBUser{
		Id:     "user1",
		APIKey: "apikey1",
		// no pub keys
	}
	s.NoError(user0.Insert(s.T().Context()))
	s.NoError(user1.Insert(s.T().Context()))

	s.rm = makeDeleteKeys()
}

func (s *UserConnectorDeleteSuite) TestDeleteSSHKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user0", PubKeys: []user.PubKey{{Name: "user0_pubkey0"}, {Name: "user0_pubkey1"}}})

	s.rm.(*keysDeleteHandler).keyName = "user0_pubkey0"
	resp := s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	user0, err := user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)
	s.Len(user0.PubKeys, 1)

	s.rm.(*keysDeleteHandler).keyName = "user0_pubkey1"
	resp = s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	user0, err = user.FindOneById(s.T().Context(), "user0")
	s.NoError(err)
	s.Empty(user0.PubKeys)
}

func (s *UserConnectorDeleteSuite) TestDeleteSSHKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*keysDeleteHandler).keyName = "keythatdoesntexist"
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *UserConnectorDeleteSuite) TestDeleteSSHKeysWithNoUserFails() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())
	})
}

func TestUserConnectorDeleteSuite(t *testing.T) {
	s := new(UserConnectorDeleteSuite)
	suite.Run(t, s)
}
