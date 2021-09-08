package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
)

type hookSuite struct {
	suite.Suite
}

func TestGithubHookSuite(t *testing.T) {
	suite.Run(t, new(hookSuite))
}

func (s *hookSuite) SetupTest() {
	s.NoError(db.Clear(GithubHooksCollection))
}

func (s *hookSuite) TestInsert() {
	hook := GithubHook{
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
	s.Equal("GithubHook ID must not be 0", err.Error())
}

func (s *hookSuite) TestFind() {
	s.TestInsert()

	hook, err := FindGithubHook("", "")
	s.NoError(err)
	s.Nil(hook)

	hook, err = FindGithubHook("evergreen-ci", "")
	s.NoError(err)
	s.Nil(hook)

	hook, err = FindGithubHook("", "evergreen")
	s.NoError(err)
	s.Nil(hook)

	hook, err = FindGithubHook("doesntexist", "evergreen")
	s.NoError(err)
	s.Nil(hook)

	hook, err = FindGithubHook("evergreen-ci", "evergreen")
	s.NoError(err)
	s.NotNil(hook)
	s.Equal(1, hook.HookID)
	s.Equal("evergreen-ci", hook.Owner)
	s.Equal("evergreen", hook.Repo)
}

func (s *hookSuite) TestRemoveGithubHook() {
	s.Error(RemoveGithubHook(1))

	hook := &GithubHook{
		HookID: 1,
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
	}
	s.NoError(hook.Insert())

	s.NoError(RemoveGithubHook(1))
	hook, err := FindGithubHookByID(1)
	s.NoError(err)
	s.Nil(hook)
}

func (s *hookSuite) TestFindGithubHookByID() {
	hook, err := FindGithubHookByID(1)
	s.NoError(err)
	s.Nil(hook)

	hook = &GithubHook{
		HookID: 1,
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
	}
	s.NoError(hook.Insert())
	hook, err = FindGithubHookByID(1)
	s.NoError(err)
	s.NotNil(hook)
}
