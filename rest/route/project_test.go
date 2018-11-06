package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for get projects route

type ProjectGetSuite struct {
	data  data.MockProjectConnector
	sc    *data.MockConnector
	route *projectGetHandler

	suite.Suite
}

func TestProjectGetSuite(t *testing.T) {
	suite.Run(t, new(ProjectGetSuite))
}

func (s *ProjectGetSuite) SetupSuite() {
	s.data = data.MockProjectConnector{
		CachedProjects: []serviceModel.ProjectRef{
			{Identifier: "projectA"},
			{Identifier: "projectB"},
			{Identifier: "projectC"},
			{Identifier: "projectD"},
			{Identifier: "projectE"},
			{Identifier: "projectF"},
		},
	}

	s.sc = &data.MockConnector{
		URL:                  "https://evergreen.example.net",
		MockProjectConnector: s.data,
	}
}

func (s *ProjectGetSuite) SetupTest() {
	s.route = &projectGetHandler{sc: s.sc}
}

func (s *ProjectGetSuite) TestPaginatorShouldErrorIfNoResults() {
	s.route.key = "zzz"
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.Equal(http.StatusNotFound, resp.Status())
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "no projects found")
}

func (s *ProjectGetSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	s.route.key = "projectC"
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	payload := resp.Data().([]interface{})

	s.Len(payload, 1)
	s.Equal(model.ToAPIString("projectC"), (payload[0]).(*model.APIProject).Identifier)

	pageData := resp.Pages()
	s.Nil(pageData.Prev)
	s.NotNil(pageData.Next)

	s.Equal("projectD", pageData.Next.Key)
}

func (s *ProjectGetSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	s.route.key = "projectA"
	s.route.limit = 100

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	payload := resp.Data().([]interface{})

	s.Len(payload, 6)
	s.Equal(model.ToAPIString("projectA"), (payload[0]).(*model.APIProject).Identifier, payload[0])
	s.Equal(model.ToAPIString("projectB"), (payload[1]).(*model.APIProject).Identifier, payload[1])

	s.Nil(resp.Pages())
}

func (s *ProjectGetSuite) TestGetRecentVersions() {
	getVersions := makeFetchProjectVersions(s.sc)
	ctx := context.Background()

	// valid request with defaults
	request, err := http.NewRequest("GET", "/projects/projectA/recent_versions", bytes.NewReader(nil))
	s.NoError(err)
	s.NoError(getVersions.Parse(ctx, request))

	// invalid limit
	request, err = http.NewRequest("GET", "/projects/projectA/recent_versions?limit=asdf", bytes.NewReader(nil))
	s.NoError(err)
	s.EqualError(getVersions.Parse(ctx, request), "400 (Bad Request): Invalid limit")

	// invalid offset
	request, err = http.NewRequest("GET", "/projects/projectA/recent_versions?offset=idk", bytes.NewReader(nil))
	s.NoError(err)
	s.EqualError(getVersions.Parse(ctx, request), "400 (Bad Request): Invalid offset")
}

////////////////////////////////////////////////////////////////////////
//
// Tests project create route
type ProjectCreateSuite struct {
	sc *data.MockConnector
	h  *projectCreateHandler

	suite.Suite
}

func TestProjectCreateSuite(t *testing.T) {
	suite.Run(t, new(ProjectCreateSuite))
}

func (s *ProjectCreateSuite) SetupTest() {
	s.sc = &data.MockConnector{}
	s.h = &projectCreateHandler{sc: s.sc}
}

func (s *ProjectCreateSuite) TestRoute() {
	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"identifier":  "id",
		"branch_name": "branch",
	})

	payload := bytes.NewBuffer(payloadBytes)

	r, err := http.NewRequest(
		"PUT",
		"https://evergreen.mongodb.com/rest/v2/projects/",
		payload,
	)

	s.Require().NoError(err)
	ctx := context.Background()
	err = s.h.Parse(ctx, r)
	s.NoError(err)
	s.Equal("id", model.FromAPIString(s.h.projectRef.Identifier))
	s.Equal("branch", model.FromAPIString(s.h.projectRef.Branch))

	resp := s.h.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusCreated, resp.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests project update route
type ProjectUpdateSuite struct {
	sc *data.MockConnector
	h  *projectUpdateHandler

	suite.Suite
}

func TestProjectUpdateSuite(t *testing.T) {
	suite.Run(t, new(ProjectUpdateSuite))
}

func (s *ProjectUpdateSuite) SetupTest() {
	s.sc = &data.MockConnector{}
	s.h = &projectUpdateHandler{sc: s.sc}
}

func (s *ProjectUpdateSuite) TestRoute() {
	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"identifier":  "id",
		"branch_name": "branch",
	})

	payload := bytes.NewBuffer(payloadBytes)

	ctx := context.Background()

	r, err := http.NewRequest(
		"PATCH",
		"https://evergreen.mongodb.com/rest/v2/projects/actual_id",
		payload,
	)
	r = r.WithContext(ctx)
	// TODO this requires newer mux
	//r = mux.SetURLVars(r, map[string]string{"project_id": "actual_id"})

	s.Require().NoError(err)
	err = s.h.Parse(ctx, r)
	s.NoError(err)
	// TODO blocked by mux update
	//s.Equal("actual_id", model.FromAPIString(s.h.projectRef.Identifier))
	s.Equal("branch", model.FromAPIString(s.h.projectRef.Branch))
	s.Equal(2, len(*s.h.keys))
	s.Subset([]string{"identifier", "branch_name"}, *s.h.keys)

	resp := s.h.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
}
