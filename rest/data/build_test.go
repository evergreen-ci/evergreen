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

type BuildConnectorSuite struct {
	ctx Connector
	suite.Suite
}

func TestBuildConnectorSuite(t *testing.T) {
	s := new(BuildConnectorSuite)
	s.ctx = &DBConnector{}

	testutil.ConfigureIntegrationTest(t, testConfig, "TestBuildConnectorSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	build1 := &build.Build{Id: "build1"}
	build2 := &build.Build{Id: "build2"}
	pr := &model.ProjectRef{Repo: "project", Identifier: "branch"}

	assert.NoError(t, build1.Insert())
	assert.NoError(t, build2.Insert())
	assert.NoError(t, pr.Insert())

	suite.Run(t, s)
}

func TestMockBuildConnectorSuite(t *testing.T) {
	s := new(BuildConnectorSuite)
	s.ctx = &MockConnector{MockBuildConnector: MockBuildConnector{
		CachedBuilds:   []build.Build{{Id: "build1"}, {Id: "build2"}},
		CachedProjects: make(map[string]*model.ProjectRef),
	}}
	s.ctx.(*MockConnector).CachedProjects["branch"] = &model.ProjectRef{Repo: "project", Identifier: "branch"}
	suite.Run(t, s)
}

func (s *BuildConnectorSuite) TestFindById() {
	b, err := s.ctx.FindBuildById("build1")
	s.NoError(err)
	s.NotNil(b)
	s.Equal("build1", b.Id)

	b, err = s.ctx.FindBuildById("build2")
	s.NoError(err)
	s.NotNil(b)
	s.Equal("build2", b.Id)
}

func (s *BuildConnectorSuite) TestFindByIdFail() {
	b, err := s.ctx.FindBuildById("build3")
	s.Error(err)
	s.Nil(b)
}

func (s *BuildConnectorSuite) TestFindProjByBranch() {
	r, err := s.ctx.FindProjectByBranch("branch")
	s.NoError(err)
	s.NotNil(r)
	s.Equal("project", r.Repo)
}

func (s *BuildConnectorSuite) TestFindProjByBranchFail() {
	r, err := s.ctx.FindProjectByBranch("notbranch")
	s.NoError(err)
	s.Nil(r)
}
