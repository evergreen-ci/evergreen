package route

import (
	"testing"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for get projects route

type ProjectGetSuite struct {
	sc        *data.MockConnector
	data      data.MockProjectConnector
	paginator PaginatorFunc

	suite.Suite
}

func TestProjectGetSuite(t *testing.T) {
	suite.Run(t, new(ProjectGetSuite))
}

func (s *ProjectGetSuite) SetupSuite() {
	varAMap := map[string]string{
		"x": "a",
		"y": "b",
	}
	varBMap := map[string]string{
		"x": "a",
		"y": "b",
	}
	varA := serviceModel.ProjectVars{
		Id:   "projectA",
		Vars: varAMap,
		PrivateVars: map[string]bool{
			"x": true,
		},
	}
	varB := serviceModel.ProjectVars{
		Id:   "projectB",
		Vars: varBMap,
		PrivateVars: map[string]bool{
			"y": true,
		},
	}
	s.data = data.MockProjectConnector{
		CachedProjects: []serviceModel.ProjectRef{
			{Identifier: "projectA"},
			{Identifier: "projectB"},
			{Identifier: "projectC"},
			{Identifier: "projectD"},
			{Identifier: "projectE"},
			{Identifier: "projectF"},
		},
		CachedVars: []*serviceModel.ProjectVars{&varA, &varB},
	}
	s.paginator = projectPaginator
	s.sc = &data.MockConnector{
		MockProjectConnector: s.data,
	}
}

func (s *ProjectGetSuite) TestPaginatorShouldErrorIfNoResults() {
	rd, err := executeProjectRequest("zzz", 1, s.sc)
	s.Error(err)
	s.NotNil(rd)
	s.Len(rd.Result, 0)
	s.Contains(err.Error(), "no projects found")
}

func (s *ProjectGetSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	rd, err := executeProjectRequest("projectC", 2, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 2)
	s.Equal(model.APIString("projectC"), (rd.Result[0]).(*model.APIProject).Identifier)
	s.Equal(model.APIString("projectD"), (rd.Result[1]).(*model.APIProject).Identifier)

	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.NotNil(pageData.Prev)
	s.NotNil(pageData.Next)

	s.Equal("projectE", pageData.Next.Key)
	s.Equal("projectA", pageData.Prev.Key)
}

func (s *ProjectGetSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	rd, err := executeProjectRequest("projectA", 100, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 6)
	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.Nil(pageData.Prev)
	s.Nil(pageData.Next)
}

func (s *ProjectGetSuite) TestPaginatorAttachesVars() {
	rd, err := executeProjectRequest("projectA", 1, s.sc)
	s.NoError(err)
	s.NotNil(rd)

	s.Len(rd.Result, 1)
	s.Equal("", (rd.Result[0]).(*model.APIProject).Vars["x"])
	s.Equal("b", (rd.Result[0]).(*model.APIProject).Vars["y"])

	rd, err = executeProjectRequest("projectB", 1, s.sc)
	s.NoError(err)
	s.NotNil(rd)

	s.Len(rd.Result, 1)
	s.Equal("a", (rd.Result[0]).(*model.APIProject).Vars["x"])
	s.Equal("", (rd.Result[0]).(*model.APIProject).Vars["y"])
}

func executeProjectRequest(key string, limit int, sc *data.MockConnector) (ResponseData, error) {
	rm := getProjectRouteManager("", 2)
	pe := (rm.Methods[0].RequestHandler).(*projectGetHandler)
	pe.Args = projectGetArgs{}
	pe.key = key
	pe.limit = limit

	return pe.Execute(nil, sc)
}
