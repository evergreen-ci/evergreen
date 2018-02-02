package data

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/suite"
)

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

type DBUserConnectorSuite struct {
	suite.Suite
	sc       *DBConnector
	numUsers int
	users    []*user.DBUser
}

func (s *DBUserConnectorSuite) SetupTest() {
	s.NoError(db.Clear(user.Collection))
	s.sc = &DBConnector{}
	s.numUsers = 10

	for i := 0; i < s.numUsers; i++ {
		testUser := &user.DBUser{
			Id:     fmt.Sprintf("user_%d", i),
			APIKey: fmt.Sprintf("apikey_%d", i),
			PubKeys: []user.PubKey{
				{
					Name: fmt.Sprintf("user_%d_0", i),
					Key:  "ssh-rsa 12345",
				},
			},
		}
		s.NoError(testUser.Insert())
		s.users = append(s.users, testUser)
	}
}

func (s *DBUserConnectorSuite) TeardownTest() {
	s.NoError(db.Clear(user.Collection))
}

func (s *DBUserConnectorSuite) TestFindUserById() {
	for i := 0; i < s.numUsers; i++ {
		found, err := s.sc.FindUserById(fmt.Sprintf("user_%d", i))
		s.NoError(err)
		s.Equal(found.GetAPIKey(), fmt.Sprintf("apikey_%d", i))
	}

	found, err := s.sc.FindUserById("fake_user")
	s.Nil(found)
	s.NoError(err)
}

func (s *DBUserConnectorSuite) TestDeletePublicKey() {
	for _, u := range s.users {
		s.NoError(s.sc.DeletePublicKey(u, u.Id+"_0"))

		dbUser, err := user.FindOne(user.ById(u.Id))
		s.NoError(err)
		s.Len(dbUser.PubKeys, 0)
	}
}

func TestDBUserConnector(t *testing.T) {
	s := &DBUserConnectorSuite{}
	suite.Run(t, s)
}
