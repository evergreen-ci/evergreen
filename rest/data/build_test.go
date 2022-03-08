package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch build by id route

type BuildConnectorFetchByIdSuite struct {
	ctx DBBuildConnector
	suite.Suite
}

func TestBuildConnectorFetchByIdSuite(t *testing.T) {
	s := new(BuildConnectorFetchByIdSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	suite.Run(t, s)
}

func (s *BuildConnectorFetchByIdSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, model.ProjectRefCollection, model.VersionCollection))
	vId := "v"
	version := &model.Version{Id: vId}
	builds := []build.Build{
		{Id: "build1", Version: vId},
		{Id: "build2", Version: vId},
	}
	s.NoError(version.Insert())
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
	projRef := model.ProjectRef{Repo: "project", Id: "branch"}
	s.NoError(projRef.Insert())
}

func (s *BuildConnectorFetchByIdSuite) TestFindById() {
	b, err := s.ctx.FindBuildById("build1")
	s.NoError(err)
	s.NotNil(b)
	s.Equal("build1", b.Id)

	b, err = s.ctx.FindBuildById("build2")
	s.NoError(err)
	s.NotNil(b)
	s.Equal("build2", b.Id)
}

func (s *BuildConnectorFetchByIdSuite) TestFindByIdFail() {
	b, err := s.ctx.FindBuildById("build3")
	s.Error(err)
	s.Nil(b)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change build status route

type BuildConnectorChangeStatusSuite struct {
	ctx DBBuildConnector
	suite.Suite
}

func TestBuildConnectorChangeStatusSuite(t *testing.T) {
	s := new(BuildConnectorChangeStatusSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	suite.Run(t, s)
}

func (s *BuildConnectorChangeStatusSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, model.VersionCollection))

	vId := "v"
	version := &model.Version{Id: vId}
	build1 := &build.Build{Id: "build1", Version: vId}
	build2 := &build.Build{Id: "build2", Version: vId}

	s.NoError(build1.Insert())
	s.NoError(build2.Insert())
	s.NoError(version.Insert())
}

func (s *BuildConnectorChangeStatusSuite) TestSetActivated() {
	err := s.ctx.SetBuildActivated("build1", "user1", true)
	s.NoError(err)
	b, err := s.ctx.FindBuildById("build1")
	s.NoError(err)
	s.True(b.Activated)
	s.Equal("user1", b.ActivatedBy)

	err = s.ctx.SetBuildActivated("build1", "user1", false)
	s.NoError(err)
	b, err = s.ctx.FindBuildById("build1")
	s.NoError(err)
	s.False(b.Activated)
	s.Equal("user1", b.ActivatedBy)
}

func (s *BuildConnectorChangeStatusSuite) TestSetPriority() {
	err := s.ctx.SetBuildPriority("build1", int64(2), "")
	s.NoError(err)

	err = s.ctx.SetBuildPriority("build1", int64(3), "")
	s.NoError(err)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort build by id route

type BuildConnectorAbortSuite struct {
	ctx DBBuildConnector
	suite.Suite
}

func TestBuildConnectorAbortSuite(t *testing.T) {
	s := new(BuildConnectorAbortSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	s.ctx = DBBuildConnector{}
	suite.Run(t, s)
}

func (s *BuildConnectorAbortSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, model.VersionCollection))

	vId := "v"
	version := &model.Version{Id: vId}
	build1 := &build.Build{Id: "build1", Version: vId}

	s.NoError(build1.Insert())
	s.NoError(version.Insert())
}

func (s *BuildConnectorAbortSuite) TestAbort() {
	err := s.ctx.AbortBuild("build1", "user1")
	s.NoError(err)
	b, err := build.FindOne(build.ById("build1"))
	s.NoError(err)
	s.Equal("user1", b.ActivatedBy)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart build route

type BuildConnectorRestartSuite struct {
	ctx DBBuildConnector
	suite.Suite
}

func TestBuildConnectorRestartSuite(t *testing.T) {
	s := new(BuildConnectorRestartSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	suite.Run(t, s)
}

func (s *BuildConnectorRestartSuite) SetupSuite() {
	s.ctx = DBBuildConnector{}

	s.NoError(db.ClearCollections(build.Collection, model.VersionCollection))

	vId := "v"
	version := &model.Version{Id: vId}
	build1 := &build.Build{Id: "build1", Version: vId}

	s.NoError(build1.Insert())
	s.NoError(version.Insert())
}

func (s *BuildConnectorRestartSuite) TestRestart() {
	err := s.ctx.RestartBuild("build1", "user1")
	s.NoError(err)

	err = s.ctx.RestartBuild("build1", "user1")
	s.NoError(err)
}
