package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch build by id route

type BuildConnectorFetchByIdSuite struct {
	suite.Suite
}

func TestBuildConnectorFetchByIdSuite(t *testing.T) {
	s := new(BuildConnectorFetchByIdSuite)
	suite.Run(t, s)
}

func (s *BuildConnectorFetchByIdSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, ProjectRefCollection, VersionCollection))
	vId := "v"
	version := &Version{Id: vId}
	builds := []build.Build{
		{Id: "build1", Version: vId},
		{Id: "build2", Version: vId},
	}
	s.NoError(version.Insert())
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
	projRef := ProjectRef{Repo: "project", Id: "branch"}
	s.NoError(projRef.Insert())
}

func (s *BuildConnectorFetchByIdSuite) TestFindById() {
	b, err := build.FindOneId("build1")
	s.NoError(err)
	s.NotNil(b)
	s.Equal("build1", b.Id)

	b, err = build.FindOneId("build2")
	s.NoError(err)
	s.NotNil(b)
	s.Equal("build2", b.Id)
}

func (s *BuildConnectorFetchByIdSuite) TestFindByIdFail() {
	b, err := build.FindOneId("build3")
	s.NoError(err)
	s.Nil(b)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change build status route

type BuildConnectorChangeStatusSuite struct {
	suite.Suite
}

func TestBuildConnectorChangeStatusSuite(t *testing.T) {
	s := new(BuildConnectorChangeStatusSuite)
	suite.Run(t, s)
}

func (s *BuildConnectorChangeStatusSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, VersionCollection))

	vId := "v"
	version := &Version{Id: vId}
	build1 := &build.Build{Id: "build1", Version: vId}
	build2 := &build.Build{Id: "build2", Version: vId}

	s.NoError(build1.Insert())
	s.NoError(build2.Insert())
	s.NoError(version.Insert())
}

func (s *BuildConnectorChangeStatusSuite) TestSetActivated() {
	err := SetBuildActivation("build1", true, "user1")
	s.NoError(err)
	b, err := build.FindOneId("build1")
	s.NoError(err)
	s.True(b.Activated)
	s.Equal("user1", b.ActivatedBy)

	err = SetBuildActivation("build1", false, "user1")
	s.NoError(err)
	b, err = build.FindOneId("build1")
	s.NoError(err)
	s.False(b.Activated)
	s.Equal("user1", b.ActivatedBy)
}

func (s *BuildConnectorChangeStatusSuite) TestSetPriority() {
	err := SetBuildPriority("build1", int64(2), "")
	s.NoError(err)

	err = SetBuildPriority("build1", int64(3), "")
	s.NoError(err)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort build by id route

type BuildConnectorAbortSuite struct {
	suite.Suite
}

func TestBuildConnectorAbortSuite(t *testing.T) {
	s := new(BuildConnectorAbortSuite)
	suite.Run(t, s)
}

func (s *BuildConnectorAbortSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, VersionCollection))

	vId := "v"
	version := &Version{Id: vId}
	build1 := &build.Build{Id: "build1", Version: vId}

	s.NoError(build1.Insert())
	s.NoError(version.Insert())
}

func (s *BuildConnectorAbortSuite) TestAbort() {
	err := AbortBuild("build1", "user1")
	s.NoError(err)
	b, err := build.FindOne(build.ById("build1"))
	s.NoError(err)
	s.Equal("user1", b.ActivatedBy)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart build route

type BuildConnectorRestartSuite struct {
	suite.Suite
}

func TestBuildConnectorRestartSuite(t *testing.T) {
	s := new(BuildConnectorRestartSuite)
	suite.Run(t, s)
}

func (s *BuildConnectorRestartSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, VersionCollection))

	vId := "v"
	version := &Version{Id: vId}
	build1 := &build.Build{Id: "build1", Version: vId}

	s.NoError(build1.Insert())
	s.NoError(version.Insert())
}

func (s *BuildConnectorRestartSuite) TestRestart() {
	err := RestartAllBuildTasks("build1", "user1")
	s.NoError(err)

	err = RestartAllBuildTasks("build1", "user1")
	s.NoError(err)
}
