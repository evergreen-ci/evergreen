package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/projects/{project_id}/variables

type GetVarsSuite struct {
	sc    *data.MockConnector
	route *getProjectVarsHandler

	suite.Suite
}

func TestGetVarsSuite(t *testing.T) {
	suite.Run(t, new(GetVarsSuite))
}

func (s *GetVarsSuite) SetupTest() {
	s.sc = getMockProjectsConnector()
	s.route = &getProjectVarsHandler{sc: s.sc}
}

func (s *GetVarsSuite) TestRunNonExistingId() {
	ctx := context.Background()
	s.route.projectID = "non-existent"

	resp := s.route.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

func (s *GetVarsSuite) TestRunExistingId() {
	ctx := context.Background()
	s.route.projectID = "dimoxinil"

	resp := s.route.Run(ctx)
	s.NotNil(resp.Data())
	s.Require().Equal(resp.Status(), http.StatusOK)
	vars := resp.Data().(model.APIProjectVars)
	s.Equal("green", vars.Vars["apple"])
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/projects/{project_id}/variables

type PatchVarsSuite struct {
	sc    *data.MockConnector
	route *patchProjectVarsHandler

	suite.Suite
}

func TestPatchVarsSuite(t *testing.T) {
	suite.Run(t, new(PatchVarsSuite))
}

func (s *PatchVarsSuite) SetupTest() {
	s.sc = getMockProjectsConnector()
	s.route = &patchProjectVarsHandler{sc: s.sc}
}

func (s *PatchVarsSuite) TestParse() {
	vars := model.APIProjectVars{
		Vars:         map[string]string{"tomato": "red"},
		VarsToDelete: []string{"apple"},
	}
	body, err := json.Marshal(vars)
	s.NoError(err)

	ctx := context.Background()
	request, err := http.NewRequest("PATCH", "/rest/v2/projects/dimoxinil/variables", bytes.NewReader(body))
	request = gimlet.SetURLVars(request, map[string]string{
		"project_id": "dimoxinil",
	})

	s.NoError(err)
	s.NoError(s.route.Parse(ctx, request))
	s.Equal("dimoxinil", s.route.projectID)
	s.Equal(vars, s.route.varsModel)
}

func (s *PatchVarsSuite) TestRunNonExistingId() {
	ctx := context.Background()
	s.route.projectID = "non-existent"

	// upsert
	resp := s.route.Run(ctx)
	s.Equal(resp.Status(), http.StatusOK)
}

func (s *PatchVarsSuite) TestRunExistingId() {
	ctx := context.Background()
	s.route.projectID = "dimoxinil"

	vars := model.APIProjectVars{
		Vars:         map[string]string{"tomato": "red"},
		VarsToDelete: []string{"apple"},
	}
	body, err := json.Marshal(vars)
	s.NoError(err)
	request, err := http.NewRequest("PATCH", "/rest/v2/projects/dimoxinil/variables", bytes.NewReader(body))
	request = gimlet.SetURLVars(request, map[string]string{
		"project_id": "dimoxinil",
	})

	s.NoError(err)
	s.NoError(s.route.Parse(ctx, request))

	resp := s.route.Run(ctx)
	s.NotNil(resp.Data())
	s.Require().Equal(resp.Status(), http.StatusOK)
	updatedVars := resp.Data().(model.APIProjectVars)
	_, ok := updatedVars.Vars["apple"]
	s.False(ok)
	s.Equal("red", updatedVars.Vars["tomato"])
	s.Equal("yellow", updatedVars.Vars["banana"])
}
