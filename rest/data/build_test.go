package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch build by id route

type BuildConnectorFetchByIdSuite struct {
	ctx  Connector
	mock bool
	suite.Suite
}

func TestBuildConnectorFetchByIdSuite(t *testing.T) {
	s := new(BuildConnectorFetchByIdSuite)
	s.ctx = &DBConnector{}
	s.mock = false

	testutil.ConfigureIntegrationTest(t, testConfig, "TestBuildConnectorFetchByIdSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	assert.NoError(t, db.Clear(build.Collection))

	build1 := &build.Build{Id: "build1"}
	build2 := &build.Build{Id: "build2"}
	pr := &model.ProjectRef{Repo: "project", Identifier: "branch"}

	assert.NoError(t, build1.Insert())
	assert.NoError(t, build2.Insert())
	assert.NoError(t, pr.Insert())

	suite.Run(t, s)
}

func TestMockBuildConnectorFetchByIdSuite(t *testing.T) {
	s := new(BuildConnectorFetchByIdSuite)
	s.ctx = &MockConnector{MockBuildConnector: MockBuildConnector{
		CachedBuilds:   []build.Build{{Id: "build1"}, {Id: "build2"}},
		CachedProjects: make(map[string]*model.ProjectRef),
		CachedAborted:  make(map[string]string),
	}}
	s.mock = true
	s.ctx.(*MockConnector).MockBuildConnector.CachedProjects["branch"] = &model.ProjectRef{Repo: "project", Identifier: "branch"}
	suite.Run(t, s)
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

func (s *BuildConnectorFetchByIdSuite) TestFindProjByBranch() {
	r, err := s.ctx.FindProjectByBranch("branch")
	s.NoError(err)
	s.NotNil(r)
	s.Equal("project", r.Repo)
}

func (s *BuildConnectorFetchByIdSuite) TestFindProjByBranchFail() {
	r, err := s.ctx.FindProjectByBranch("notbranch")
	s.NoError(err)
	s.Nil(r)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change build status route

type BuildConnectorChangeStatusSuite struct {
	ctx  Connector
	mock bool

	suite.Suite
}

func TestBuildConnectorChangeStatusSuite(t *testing.T) {
	s := new(BuildConnectorChangeStatusSuite)

	s.ctx = &DBConnector{}

	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorAbortByIdSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	assert.NoError(t, db.Clear(build.Collection))

	build1 := &build.Build{Id: "build1"}
	build2 := &build.Build{Id: "build2"}

	assert.NoError(t, build1.Insert())
	assert.NoError(t, build2.Insert())

	s.mock = false
	suite.Run(t, s)
}

func TestMockBuildConnectorChangeStatusSuite(t *testing.T) {
	s := new(BuildConnectorChangeStatusSuite)

	s.ctx = &MockConnector{MockBuildConnector: MockBuildConnector{
		CachedBuilds: []build.Build{
			{Id: "build1"},
			{Id: "build2"},
		},
	}}

	s.mock = true
	suite.Run(t, s)
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

func (s *BuildConnectorChangeStatusSuite) TestSetActivatedFail() {
	err := s.ctx.SetBuildActivated("zzz", "user1", true)
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *BuildConnectorChangeStatusSuite) TestSetPriority() {
	err := s.ctx.SetBuildPriority("build1", int64(2))
	s.NoError(err)

	err = s.ctx.SetBuildPriority("build1", int64(3))
	s.NoError(err)
}

func (s *BuildConnectorChangeStatusSuite) TestSetPriorityFail() {
	if s.mock {
		s.ctx.(*MockConnector).MockBuildConnector.FailOnChangePriority = true
		err := s.ctx.SetBuildPriority("build1", int64(2))
		s.Error(err)
	}
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort build by id route

type BuildConnectorAbortSuite struct {
	ctx  Connector
	mock bool
	suite.Suite
}

func TestBuildConnectorAbortSuite(t *testing.T) {
	s := new(BuildConnectorAbortSuite)
	s.ctx = &DBConnector{}
	s.mock = false

	testutil.ConfigureIntegrationTest(t, testConfig, "TestBuildConnectorAbortSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	assert.NoError(t, db.Clear(build.Collection))

	build1 := &build.Build{Id: "build1"}
	assert.NoError(t, build1.Insert())

	suite.Run(t, s)
}

func TestMockBuildConnectorAbortSuite(t *testing.T) {
	s := new(BuildConnectorAbortSuite)
	s.ctx = &MockConnector{MockBuildConnector: MockBuildConnector{
		CachedBuilds:  []build.Build{{Id: "build1"}},
		CachedAborted: make(map[string]string),
	}}
	s.mock = true
	suite.Run(t, s)
}

func (s *BuildConnectorAbortSuite) TestAbort() {
	err := s.ctx.AbortBuild("build1", "user1")
	s.NoError(err)
	if s.mock {
		s.Equal("user1", s.ctx.(*MockConnector).MockBuildConnector.CachedAborted["build1"])
	} else {
		b, err := build.FindOne(build.ById("build1"))
		s.NoError(err)
		s.Equal("user1", b.ActivatedBy)
	}
}

func (s *BuildConnectorAbortSuite) TestAbortFail() {
	if s.mock {
		s.ctx.(*MockConnector).MockBuildConnector.FailOnAbort = true
		err := s.ctx.AbortBuild("build1", "user1")
		s.Error(err)
	}
}
