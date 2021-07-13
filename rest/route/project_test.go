package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
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
	s.NoError(db.ClearCollections(serviceModel.RepoRefCollection, user.Collection))
	s.sc = getMockProjectsConnector()

	settings, err := evergreen.GetConfig()
	s.NoError(err)
	s.rm = makePatchProjectByID(s.sc, settings).(*projectIDPatchHandler)
}

func (s *ProjectPatchByIDSuite) TestParse() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})

	json := []byte(`{"private" : false}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
}

func (s *ProjectPatchByIDSuite) TestRunInValidIdentifierChange() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	json := []byte(`{"id": "Verboten"}`)
	h := s.rm.(*projectIDPatchHandler)
	h.user = &user.DBUser{Id: "me"}
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.Error(err)
	s.Contains(err.Error(), "A project's id is immutable")
}

func (s *ProjectPatchByIDSuite) TestRunInvalidNonExistingId() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	json := []byte(`{"display_name": "This is a display name"}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/non-existent", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "non-existent"})
	err := s.rm.Parse(ctx, req)
	s.Require().Error(err)
	s.Contains(err.Error(), "error finding original project")
}

func (s *ProjectPatchByIDSuite) TestRunValid() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"enabled": true, "revision": "my_revision", "variables": {"vars_to_delete": ["apple"]} }`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	vars, err := s.sc.FindProjectVarsById("dimoxinil", "", false)
	s.NoError(err)
	_, ok := vars.Vars["apple"]
	s.False(ok)
	_, ok = vars.Vars["banana"]
	s.True(ok)
}

func (s *ProjectPatchByIDSuite) TestRunWithCommitQueueEnabled() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	jsonBody := []byte(`{"enabled": true, "commit_queue": {"enabled": true}}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(http.StatusBadRequest, resp.Status())
	errResp := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal("cannot enable commit queue without a commit queue patch definition", errResp.Message)
}

func (s *ProjectPatchByIDSuite) TestGitTagVersionsEnabled() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	jsonBody := []byte(`{"enabled": true, "git_tag_versions_enabled": true, "aliases": [{"alias": "__git_tag", "git_tag": "my_git_tag", "variant": ".*", "tag": ".*"}]}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	h := s.rm.(*projectIDPatchHandler)
	s.NotNil(h.user)

	repoRef := serviceModel.RepoRef{ProjectRef: serviceModel.ProjectRef{
		Id:                    mgobson.NewObjectId().Hex(),
		Owner:                 h.originalProject.Owner,
		Repo:                  h.originalProject.Repo,
		GitTagAuthorizedUsers: []string{"special"},
		Restricted:            utility.FalsePtr(),
	}}
	s.NoError(repoRef.Add(nil))

	jsonBody = []byte(`{"enabled": true, "use_repo_settings": true, "git_tag_versions_enabled": true, "aliases": [{"alias": "__git_tag", "git_tag": "my_git_tag", "variant": ".*", "tag": ".*"}]}`)
	req, _ = http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err = s.rm.Parse(ctx, req)
	s.NoError(err)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(http.StatusOK, resp.Status())

	// verify that the repo fields weren't saved with the branch
	p, err := s.sc.FindProjectById("dimoxinil", false)
	s.NoError(err)
	s.Require().NotNil(p)
	s.Empty(p.GitTagAuthorizedUsers)
	s.Nil(p.Restricted)
}

func (s *ProjectPatchByIDSuite) TestUseRepoSettings() {
	ctx := context.Background()
	jsonBody := []byte(`{"use_repo_settings": true, "admins": ["me"]}`)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	u := &user.DBUser{Id: "me"}
	s.NoError(u.Insert())

	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	h := s.rm.(*projectIDPatchHandler)
	s.NotNil(h.user)
	s.NotNil(h.originalProject)
	repoRef, err := serviceModel.FindRepoRefByOwnerAndRepo(h.originalProject.Owner, h.originalProject.Repo)
	s.NoError(err)
	s.Nil(repoRef)
	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err := s.sc.FindProjectById("dimoxinil", true)
	s.NoError(err)
	s.True(p.UseRepoSettings)
	s.NotEmpty(p.RepoRefId)
	s.Contains(p.Admins, "me")

	u, err = user.FindOneById("me")
	s.NoError(err)
	s.NotNil(u)
	s.Contains(u.Roles(), serviceModel.GetViewRepoRole(p.RepoRefId))
	s.Contains(u.Roles(), serviceModel.GetRepoAdminRole(p.RepoRefId))

	repoRef, err = serviceModel.FindRepoRefByOwnerAndRepo(h.originalProject.Owner, h.originalProject.Repo)
	s.NoError(err)
	s.NotNil(repoRef)
}

func (s *ProjectPatchByIDSuite) TestFilesIgnoredFromCache() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	h := s.rm.(*projectIDPatchHandler)
	h.user = &user.DBUser{Id: "me"}

	jsonBody := []byte(`{"files_ignored_from_cache": []}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err := s.sc.FindProjectById("dimoxinil", true)
	s.NoError(err)
	s.False(p.FilesIgnoredFromCache == nil)
	s.Len(p.FilesIgnoredFromCache, 0)
}

func (s *ProjectPatchByIDSuite) TestPatchTriggerAliases() {
	s.NoError(db.Clear(serviceModel.ProjectRefCollection))
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	h := s.rm.(*projectIDPatchHandler)
	h.user = &user.DBUser{Id: "me"}

	jsonBody := []byte(`{"patch_trigger_aliases": [{"child_project": "child", "task_specifiers": [ {"task_regex": ".*", "variant_regex": ".*" }]}]}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.NoError(s.rm.Parse(ctx, req))
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusBadRequest) // child project doesn't exist yet

	childProject := serviceModel.ProjectRef{
		Id:         "firstborn",
		Identifier: "child",
	}
	s.NoError(childProject.Insert())
	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err := s.sc.FindProjectById("dimoxinil", true)
	s.NoError(err)
	s.False(p.PatchTriggerAliases == nil)
	s.Len(p.PatchTriggerAliases, 1)
	s.Equal(p.PatchTriggerAliases[0].ChildProject, "firstborn") // saves ID

	jsonBody = []byte(`{"patch_trigger_aliases": []}`) // empty list isn't nil
	req, _ = http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.NoError(s.rm.Parse(ctx, req))
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err = s.sc.FindProjectById("dimoxinil", true)
	s.NoError(err)
	s.NotNil(p.PatchTriggerAliases)
	s.Len(p.PatchTriggerAliases, 0)

	jsonBody = []byte(`{"patch_trigger_aliases": null}`)
	req, _ = http.NewRequest("PATCH", "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.NoError(s.rm.Parse(ctx, req))
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err = s.sc.FindProjectById("dimoxinil", true)
	s.NoError(err)
	s.Nil(p.PatchTriggerAliases)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PUT /rest/v2/projects/{project_id}

type ProjectPutSuite struct {
	sc *data.MockConnector
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
				"branch_name": "main",
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
				"hidden": true,
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{})
	json := []byte(
		`{
				"owner_name": "Rembrandt Q. Einstein",
				"repo_name": "nutsandgum",
				"branch_name": "main",
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
				"hidden": true,
				"patching_disabled": true,
				"admins": ["Apu DeBeaumarchais"],
				"notify_on_failure": true
		}`)

	h := s.rm.(*projectIDPutHandler)
	h.projectName = "nutsandgum"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusCreated)

	p, err := h.sc.FindProjectById("nutsandgum", false)
	s.NoError(err)
	s.Require().NotNil(p)
	s.NotEqual("nutsandgum", p.Id)
	s.Equal("nutsandgum", p.Identifier)
}

func (s *ProjectPutSuite) TestRunExistingFails() {
	ctx := context.Background()
	json := []byte(
		`{
				"owner_name": "Rembrandt Q. Einstein",
				"repo_name": "nutsandgum",
		}`)

	h := s.rm.(*projectIDPutHandler)
	h.projectName = "dimoxinil"
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
	h.projectName = "non-existent"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

func (s *ProjectGetByIDSuite) TestRunExistingId() {
	ctx := context.Background()
	h := s.rm.(*projectIDGetHandler)
	h.projectName = "dimoxinil"

	resp := s.rm.Run(ctx)
	s.Require().NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	projectRef, ok := resp.Data().(*model.APIProjectRef)
	s.Require().True(ok)
	cachedProject := s.sc.MockProjectConnector.CachedProjects[0]
	s.Equal(cachedProject.Repo, utility.FromStringPtr(projectRef.Repo))
	s.Equal(cachedProject.Owner, utility.FromStringPtr(projectRef.Owner))
	s.Equal(cachedProject.Branch, utility.FromStringPtr(projectRef.Branch))
	s.Equal(cachedProject.Enabled, projectRef.Enabled)
	s.Equal(cachedProject.Private, projectRef.Private)
	s.Equal(cachedProject.BatchTime, projectRef.BatchTime)
	s.Equal(cachedProject.RemotePath, utility.FromStringPtr(projectRef.RemotePath))
	s.Equal(cachedProject.Id, utility.FromStringPtr(projectRef.Id))
	s.Equal(cachedProject.DisplayName, utility.FromStringPtr(projectRef.DisplayName))
	s.Equal(cachedProject.DeactivatePrevious, projectRef.DeactivatePrevious)
	s.Equal(cachedProject.TracksPushEvents, projectRef.TracksPushEvents)
	s.Equal(cachedProject.PRTestingEnabled, projectRef.PRTestingEnabled)
	s.Equal(cachedProject.CommitQueue.Enabled, projectRef.CommitQueue.Enabled)
	s.Equal(cachedProject.Hidden, projectRef.Hidden)
	s.Equal(cachedProject.PatchingDisabled, projectRef.PatchingDisabled)
	s.Equal(cachedProject.Admins, utility.FromStringPtrSlice(projectRef.Admins))
	s.Equal(cachedProject.NotifyOnBuildFailure, projectRef.NotifyOnBuildFailure)
	s.Equal(cachedProject.DisabledStatsCache, projectRef.DisabledStatsCache)
	s.Equal(cachedProject.FilesIgnoredFromCache, utility.FromStringPtrSlice(projectRef.FilesIgnoredFromCache))
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
			{Id: "projectA"},
			{Id: "projectB"},
			{Id: "projectC"},
			{Id: "projectD"},
			{Id: "projectE"},
			{Id: "projectF"},
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
	s.Equal(utility.ToStringPtr("projectC"), (payload[0]).(*model.APIProjectRef).Id)

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
	s.Equal(utility.ToStringPtr("projectA"), (payload[0]).(*model.APIProjectRef).Id, payload[0])
	s.Equal(utility.ToStringPtr("projectB"), (payload[1]).(*model.APIProjectRef).Id, payload[1])

	s.Nil(resp.Pages())
}

func (s *ProjectGetSuite) TestGetRecentVersions() {
	getVersions := makeFetchProjectVersionsLegacy(s.sc)
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
					Branch:             "main",
					Enabled:            utility.FalsePtr(),
					Private:            utility.TruePtr(),
					BatchTime:          0,
					RemotePath:         "evergreen.yml",
					Id:                 "dimoxinil",
					DisplayName:        "Dimoxinil",
					DeactivatePrevious: utility.FalsePtr(),
					TracksPushEvents:   utility.FalsePtr(),
					PRTestingEnabled:   utility.FalsePtr(),
					CommitQueue: serviceModel.CommitQueueParams{
						Enabled: utility.FalsePtr(),
					},
					Hidden:                utility.FalsePtr(),
					PatchingDisabled:      utility.FalsePtr(),
					Admins:                []string{"langdon.alger"},
					NotifyOnBuildFailure:  utility.FalsePtr(),
					DisabledStatsCache:    utility.TruePtr(),
					FilesIgnoredFromCache: []string{"ignored"},
				},
			},
			CachedVars: []*serviceModel.ProjectVars{
				{
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
	assert.NoError(db.ClearCollections(serviceModel.VersionCollection, serviceModel.ProjectRefCollection))
	const projectId = "proj"
	project := serviceModel.ProjectRef{
		Id:         projectId,
		Identifier: "something-else",
	}
	assert.NoError(project.Insert())
	v1 := serviceModel.Version{
		Id:                  "v1",
		Identifier:          projectId,
		Requester:           evergreen.AdHocRequester,
		RevisionOrderNumber: 1,
	}
	assert.NoError(v1.Insert())
	v2 := serviceModel.Version{
		Id:                  "v2",
		Identifier:          projectId,
		Requester:           evergreen.AdHocRequester,
		RevisionOrderNumber: 2,
	}
	assert.NoError(v2.Insert())
	v3 := serviceModel.Version{
		Id:                  "v3",
		Identifier:          projectId,
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 3,
	}
	assert.NoError(v3.Insert())
	v4 := serviceModel.Version{
		Id:                  "v4",
		Identifier:          projectId,
		Requester:           evergreen.AdHocRequester,
		RevisionOrderNumber: 4,
	}
	assert.NoError(v4.Insert())

	h := getProjectVersionsHandler{
		projectName: "something-else",
		sc:          &data.DBConnector{},
		opts: serviceModel.GetVersionsOptions{
			Requester: evergreen.AdHocRequester,
			Limit:     20,
		},
	}

	resp := h.Run(context.Background())
	respJson, err := json.Marshal(resp.Data())
	assert.NoError(err)
	assert.Contains(string(respJson), `"version_id":"v4"`)
	assert.NotContains(string(respJson), `"version_id":"v3"`)
}

func TestDeleteProject(t *testing.T) {
	assert.NoError(t, db.ClearCollections(
		serviceModel.ProjectRefCollection,
		serviceModel.RepoRefCollection,
		serviceModel.ProjectAliasCollection,
		serviceModel.ProjectVarsCollection,
	))

	repo := serviceModel.RepoRef{
		ProjectRef: serviceModel.ProjectRef{
			Id:      "repo_ref",
			Owner:   "mongodb",
			Repo:    "test_repo",
			Enabled: utility.TruePtr(),
		},
	}
	assert.NoError(t, repo.Upsert())

	// Projects expected to be successfully deleted
	numGoodProjects := 2
	var projects []serviceModel.ProjectRef
	for i := 0; i < numGoodProjects; i++ {
		project := serviceModel.ProjectRef{
			Id:                   fmt.Sprintf("id_%d", i),
			Owner:                "mongodb",
			Repo:                 "test_repo",
			Branch:               fmt.Sprintf("branch_%d", i),
			Enabled:              utility.TruePtr(),
			Private:              utility.TruePtr(),
			DisplayName:          fmt.Sprintf("display_%d", i),
			UseRepoSettings:      true,
			RepoRefId:            "repo_ref",
			TracksPushEvents:     utility.TruePtr(),
			PRTestingEnabled:     utility.TruePtr(),
			Admins:               []string{"admin0", "admin1"},
			NotifyOnBuildFailure: utility.TruePtr(),
		}

		projects = append(projects, project)
		require.NoError(t, project.Insert())
	}

	numAliases := 2
	var aliases []serviceModel.ProjectAlias
	for i := 0; i < numAliases; i++ {
		projAlias := serviceModel.ProjectAlias{
			ProjectID: projects[0].Id,
			Alias:     fmt.Sprintf("alias_%d", i),
			Variant:   fmt.Sprintf("variant_%d", i),
			Task:      fmt.Sprintf("task_%d", i),
		}

		aliases = append(aliases, projAlias)
		require.NoError(t, projAlias.Upsert())
	}

	projVars := serviceModel.ProjectVars{
		Id:   projects[0].Id,
		Vars: map[string]string{"hello": "world"},
	}
	_, err := projVars.Upsert()
	require.NoError(t, err)

	ctx := context.Background()
	pdh := projectDeleteHandler{
		sc: &data.MockConnector{},
	}

	// Test cases:
	// 0) Project with 2 ProjectAliases and a ProjectVars
	// 1) Project with 0 ProjectAliases and no ProjectVars
	for i := 0; i < numGoodProjects; i++ {
		pdh.projectName = projects[i].Id
		resp := pdh.Run(ctx)
		assert.Equal(t, http.StatusOK, resp.Status())

		hiddenProj, err := serviceModel.FindMergedProjectRef(projects[i].Id)
		assert.NoError(t, err)
		skeletonProj := serviceModel.ProjectRef{
			Id:              projects[i].Id,
			Owner:           repo.Owner,
			Repo:            repo.Repo,
			Branch:          projects[i].Branch,
			RepoRefId:       repo.Id,
			Enabled:         utility.FalsePtr(),
			UseRepoSettings: true,
			Hidden:          utility.TruePtr(),
		}
		assert.Equal(t, skeletonProj, *hiddenProj)

		projAliases, err := serviceModel.FindAliasesForProject(projects[i].Id)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(projAliases))

		skeletonProjVars := serviceModel.ProjectVars{
			Id: projects[i].Id,
		}
		projVars, err := serviceModel.FindOneProjectVars(projects[i].Id)
		assert.NoError(t, err)
		assert.Equal(t, skeletonProjVars, *projVars)
	}

	// Testing projects ineligible for deletion
	// Project that's already hidden
	pdh.projectName = projects[0].Id
	resp := pdh.Run(ctx)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	// Non-existent project
	pdh.projectName = "bad_project"
	resp = pdh.Run(ctx)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	// Project with UseRepoSettings == false
	badProject := serviceModel.ProjectRef{
		Id:              "bad_project",
		UseRepoSettings: false,
	}
	require.NoError(t, badProject.Insert())
	pdh.projectName = badProject.Id
	resp = pdh.Run(ctx)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PUT /rest/v2/projects/variables/rotate

type ProjectPutRotateSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestProjectPutRotateSuite(t *testing.T) {
	suite.Run(t, new(ProjectPutRotateSuite))
}

func (s *ProjectPutRotateSuite) SetupTest() {
	s.sc = getMockProjectsConnector()
	s.rm = makeProjectVarsPut(s.sc).(*projectVarsPutHandler)
}

func (s *ProjectPutRotateSuite) TestRotateProjectVars() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})

	dryRunTrue := []byte(
		`{
				"to_replace": "yellow",
				"replacement": "brown",
				"dry_run": true
		}`)

	dryRunFalse := []byte(
		`{
				"to_replace": "yellow",
				"replacement": "brown"
		}`)

	req, _ := http.NewRequest("PUT", "http://example.com/api/rest/v2/projects/variables/rotate", bytes.NewBuffer(dryRunTrue))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	s.Equal("yellow", s.sc.CachedVars[0].Vars["banana"])

	req, _ = http.NewRequest("PUT", "http://example.com/api/rest/v2/projects/variables/rotate", bytes.NewBuffer(dryRunFalse))
	err = s.rm.Parse(ctx, req)
	s.NoError(err)
	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	s.Equal("brown", s.sc.CachedVars[0].Vars["banana"])
}
