package github_hook

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type hookSuite struct {
	suite.Suite
}

func TestHookSuite(t *testing.T) {
	suite.Run(t, new(hookSuite))
}

func (s *hookSuite) SetupSuite() {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func (s *hookSuite) SetupTest() {
	s.NoError(db.Clear(Collection))
}

func (s *hookSuite) TestInsert() {
	hook := Hook{
		HookID: 1,
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
	}

	s.NoError(hook.Insert())

	hook.Owner = ""
	err := hook.Insert()
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())

	hook.Owner = "evergreen-ci"
	hook.Repo = ""
	err = hook.Insert()
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())

	hook.Repo = "evergreen"
	hook.HookID = 0
	err = hook.Insert()
	s.Error(err)
	s.Equal("Hook ID must not be 0", err.Error())
}

func (s *hookSuite) TestFind() {
	s.TestInsert()

	hook, err := FindHook("", "")
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())
	s.Nil(hook)

	hook, err = FindHook("evergreen-ci", "")
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())
	s.Nil(hook)

	hook, err = FindHook("", "evergreen")
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())
	s.Nil(hook)

	hook, err = FindHook("doesntexist", "evergreen")
	s.NoError(err)
	s.Nil(hook)

	hook, err = FindHook("evergreen-ci", "evergreen")
	s.NoError(err)
	s.NotNil(hook)
	s.Equal(1, hook.HookID)
	s.Equal("evergreen-ci", hook.Owner)
	s.Equal("evergreen", hook.Repo)
}
