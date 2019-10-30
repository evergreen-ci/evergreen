package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/projects/{project_id}

type ProjectPatchByIDSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestProjectPatchSuite(t *testing.T) {

	suite.Run(t, new(ProjectPatchByIDSuite))
}

func (s *ProjectPatchByIDSuite) SetupTest() {
	s.sc = getMockProjectsConnector()

	settings, err := evergreen.GetConfig()
	s.NoError(err)
	s.rm = makePatchProjectByID(s.sc, settings).(*projectIDPatchHandler)
}

func (s *ProjectPatchByIDSuite) TestParse() {
	ctx := context.Background()

	json := []byte(`{"private" : false}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil?revision=my-revision", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.Equal(json, s.rm.(*projectIDPatchHandler).body)
}

func (s *ProjectPatchByIDSuite) TestRunInValidIdentifierChange() {
	ctx := context.Background()
	json := []byte(`{"identifier": "Verboten"}`)
	h := s.rm.(*projectIDPatchHandler)
	h.projectID = "dimoxinil"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)

	gimlet := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(gimlet.Message, fmt.Sprintf("A project's id is immutable; cannot rename project '%s'", h.projectID))
}

func (s *ProjectPatchByIDSuite) TestRunInvalidNonExistingId() {
	ctx := context.Background()
	json := []byte(`{"display_name": "This is a display name"}`)
	h := s.rm.(*projectIDPatchHandler)
	h.projectID = "non-existent"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

func (s *ProjectPatchByIDSuite) TestRunValid() {
	ctx := context.Background()
	json := []byte(`{"enabled": true, "revision": "my_revision", "variables": {"vars_to_delete": ["apple"]} }`)
	h := s.rm.(*projectIDPatchHandler)
	h.projectID = "dimoxinil"
	h.body = json
	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	vars, err := s.sc.FindProjectVarsById("dimoxinil")
	s.NoError(err)
	_, ok := vars.Vars["apple"]
	s.False(ok)
	_, ok = vars.Vars["banana"]
	s.True(ok)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PUT /rest/v2/projects/{project_id}

type ProjectPutSuite struct {
	sc *data.MockConnector
	// data data.MockProjectConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestProjectPutSuite(t *testing.T) {

	suite.Run(t, new(ProjectPutSuite))
}

func (s *ProjectPutSuite) SetupTest() {
	s.sc = getMockProjectsConnector()
	s.rm = makePutProjectByID(s.sc).(*projectIDPutHandler)
}

func (s *ProjectPutSuite) TestParse() {
	ctx := context.Background()
	json := []byte(
		`{
				"owner_name": "Rembrandt Q. Einstein",
				"repo_name": "nutsandgum",
				"branch_name": "master",
				"repo_kind": "github",
				"enabled": false,
				"private": true,
				"batch_time": 0,
				"remote_path": "evergreen.yml",
				"display_name": "Nuts and Gum: together at last!",
				"local_config": "",
				"deactivate_previous": true,
				"tracks_push_events": true,
				"pr_testing_enabled": true,
				"commitq_enabled": true,
				"tracked": false,
				"patching_disabled": true,
				"admins": ["Apu DeBeaumarchais"],
				"notify_on_failure": true
		}`)

	req, _ := http.NewRequest("PUT", "http://example.com/api/rest/v2/projects/nutsandgum", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
}

func (s *ProjectPutSuite) TestRunNewWithValidEntity() {
	ctx := context.Background()
	json := []byte(
		`{
				"owner_name": "Rembrandt Q. Einstein",
				"repo_name": "nutsandgum",
				"branch_name": "master",
				"repo_kind": "github",
				"enabled": false,
				"private": true,
				"batch_time": 0,
				"remote_path": "evergreen.yml",
				"display_name": "Nuts and Gum: together at last!",
				"local_config": "",
				"deactivate_previous": true,
				"tracks_push_events": true,
				"pr_testing_enabled": true,
				"commitq_enabled": true,
				"tracked": false,
				"patching_disabled": true,
				"admins": ["Apu DeBeaumarchais"],
				"notify_on_failure": true
		}`)

	h := s.rm.(*projectIDPutHandler)
	h.projectID = "nutsandgum"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusCreated)
}

func (s *ProjectPutSuite) TestRunExistingFails() {
	ctx := context.Background()
	json := []byte(
		`{
				"owner_name": "Rembrandt Q. Einstein",
				"repo_name": "nutsandgum",
		}`)

	h := s.rm.(*projectIDPutHandler)
	h.projectID = "dimoxinil"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusBadRequest)

}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/projects/{project_id}

type ProjectGetByIDSuite struct {
	sc *data.MockConnector
	// data data.MockProjectConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestProjectGetByIDSuite(t *testing.T) {

	suite.Run(t, new(ProjectGetByIDSuite))
}

func (s *ProjectGetByIDSuite) SetupTest() {
	s.sc = getMockProjectsConnector()
	s.rm = makeGetProjectByID(s.sc).(*projectIDGetHandler)
}

func (s *ProjectGetByIDSuite) TestRunNonExistingId() {
	ctx := context.Background()
	h := s.rm.(*projectIDGetHandler)
	h.projectID = "non-existent"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

func (s *ProjectGetByIDSuite) TestRunExistingId() {
	ctx := context.Background()
	h := s.rm.(*projectIDGetHandler)
	h.projectID = "dimoxinil"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	s.Equal(model.ToAPIString("dimoxinil"), resp.Data().(*model.APIProjectRef).Identifier)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/projects

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
	s.Equal(model.ToAPIString("projectC"), (payload[0]).(*model.APIProjectRef).Identifier)

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
	s.Equal(model.ToAPIString("projectA"), (payload[0]).(*model.APIProjectRef).Identifier, payload[0])
	s.Equal(model.ToAPIString("projectB"), (payload[1]).(*model.APIProjectRef).Identifier, payload[1])

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

func getMockProjectsConnector() *data.MockConnector {
	connector := data.MockConnector{
		MockProjectConnector: data.MockProjectConnector{
			CachedProjects: []serviceModel.ProjectRef{
				{
					Owner:              "dimoxinil",
					Repo:               "dimoxinil-enterprise-repo",
					Branch:             "master",
					RepoKind:           "github",
					Enabled:            false,
					Private:            true,
					BatchTime:          0,
					RemotePath:         "evergreen.yml",
					Identifier:         "dimoxinil",
					DisplayName:        "Dimoxinil",
					DeactivatePrevious: false,
					TracksPushEvents:   false,
					PRTestingEnabled:   false,
					CommitQueue: serviceModel.CommitQueueParams{
						Enabled: false,
					},
					Tracked:              true,
					PatchingDisabled:     false,
					Admins:               []string{"langdon.alger"},
					NotifyOnBuildFailure: false,
				},
			},
			CachedVars: []*serviceModel.ProjectVars{
				&serviceModel.ProjectVars{
					Id:   "dimoxinil",
					Vars: map[string]string{"apple": "green", "banana": "yellow"},
				},
			},
		},
	}
	return &connector
}

func TestGetProjectVersions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(serviceModel.VersionCollection))
	const projectName = "proj"
	v1 := serviceModel.Version{
		Id:                  "v1",
		Identifier:          projectName,
		Requester:           evergreen.AdHocRequester,
		RevisionOrderNumber: 1,
	}
	assert.NoError(v1.Insert())
	v2 := serviceModel.Version{
		Id:                  "v2",
		Identifier:          projectName,
		Requester:           evergreen.AdHocRequester,
		RevisionOrderNumber: 2,
	}
	assert.NoError(v2.Insert())
	v3 := serviceModel.Version{
		Id:                  "v3",
		Identifier:          projectName,
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 3,
	}
	assert.NoError(v3.Insert())
	v4 := serviceModel.Version{
		Id:                  "v4",
		Identifier:          projectName,
		Requester:           evergreen.AdHocRequester,
		RevisionOrderNumber: 4,
	}
	assert.NoError(v4.Insert())

	h := getProjectVersionsHandler{
		projectID: projectName,
		requester: evergreen.AdHocRequester,
		sc:        &data.DBConnector{},
		limit:     20,
	}

	resp := h.Run(context.Background())
	respJson, err := json.Marshal(resp.Data())
	assert.NoError(err)
	assert.Contains(string(respJson), `"version_id":"v4"`)
	assert.NotContains(string(respJson), `"version_id":"v3"`)
}
