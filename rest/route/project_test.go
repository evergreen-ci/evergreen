package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/projects/{project_id}

type ProjectPatchByIDSuite struct {
	rm     gimlet.RouteHandler
	env    evergreen.Environment
	cancel context.CancelFunc

	suite.Suite
}

func TestProjectPatchSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &ProjectPatchByIDSuite{
		env: testutil.NewEnvironment(ctx, t),
	}
	suite.Run(t, s)
}

func (s *ProjectPatchByIDSuite) SetupTest() {
	s.NoError(db.ClearCollections(serviceModel.RepoRefCollection, user.Collection, serviceModel.ProjectRefCollection, serviceModel.ProjectVarsCollection, fakeparameter.Collection, serviceModel.RepositoriesCollection, serviceModel.ProjectAliasCollection,
		evergreen.ScopeCollection, evergreen.RoleCollection, evergreen.ConfigCollection))
	user := user.DBUser{
		Id:          "langdon.alger",
		SystemRoles: []string{"admin"},
	}
	s.NoError(user.Insert())
	s.NoError(getTestProjectRef().Add(&user))
	project2 := getTestProjectRef()
	project2.Id = "project2"
	project2.Identifier = "project2"
	s.NoError(project2.Add(&user))

	_, err := getTestVar().Upsert()
	s.NoError(err)
	aliases := getTestAliases()
	for _, alias := range aliases {
		s.NoError(alias.Upsert())
	}
	s.NoError(db.Insert(serviceModel.RepositoriesCollection, serviceModel.Repository{
		Project:      "dimoxinil",
		LastRevision: "something",
	}))

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	env := testutil.NewEnvironment(ctx, s.T())
	settings := env.Settings()
	settings.GithubOrgs = []string{getTestProjectRef().Owner}
	projectSetting := evergreen.ProjectCreationConfig{
		TotalProjectLimit: 1,
		RepoProjectLimit:  1,
	}
	s.NoError(projectSetting.Set(ctx))
	s.rm = makePatchProjectByID(settings).(*projectIDPatchHandler)
	projectAdminRole := gimlet.Role{
		ID:    "dimoxinil",
		Scope: "project_scope",
		Permissions: gimlet.Permissions{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
		},
	}
	roleManager := s.env.RoleManager()
	s.NoError(roleManager.UpdateRole(projectAdminRole))
	adminScope := gimlet.Scope{
		ID:        "project_scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"dimoxinil", "other_project", "branch_project"},
	}
	s.NoError(roleManager.AddScope(adminScope))
}

func (s *ProjectPatchByIDSuite) TearDownTest() {
	s.cancel()
}

func (s *ProjectPatchByIDSuite) TestParse() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})

	json := []byte(`{"private" : false}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
}

func (s *ProjectPatchByIDSuite) TestRunInvalidIdentifierChange() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	json := []byte(`{"id": "Verboten"}`)
	h := s.rm.(*projectIDPatchHandler)
	h.user = &user.DBUser{Id: "me"}
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.Error(err)
	s.Contains(err.Error(), "project ID is immutable")
}

func (s *ProjectPatchByIDSuite) TestRunInvalidNonExistingId() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	json := []byte(`{"display_name": "This is a display name"}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/non-existent", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "non-existent"})
	err := s.rm.Parse(ctx, req)
	s.Require().Error(err)
	s.Contains(err.Error(), "finding original project")
}

func (s *ProjectPatchByIDSuite) TestRunProjectCreateValidationFail() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	json := []byte(`{"enabled": true}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/project2", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "project2"})
	err = s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusBadRequest)
}

func (s *ProjectPatchByIDSuite) TestRunValid() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"enabled": true, "revision": "my_revision", "variables": {"vars_to_delete": ["apple"]} }`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(json))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	vars, err := data.FindProjectVarsById("dimoxinil", "", false)
	s.NoError(err)
	_, ok := vars.Vars["apple"]
	s.False(ok)
	_, ok = vars.Vars["banana"]
	s.True(ok)
}

func (s *ProjectPatchByIDSuite) TestRunWithCommitQueueEnabled() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	jsonBody := []byte(`{"enabled": true, "commit_queue": {"enabled": true}}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(http.StatusBadRequest, resp.Status())
	errResp := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal("cannot enable commit queue without first enabling GitHub webhooks", errResp.Message)
}

func (s *ProjectPatchByIDSuite) TestRunWithValidBbConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	jsonBody := []byte(`{"enabled": true, "build_baron_settings": {"ticket_create_project": "EVG", "ticket_search_projects": ["EVG"]}}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(http.StatusOK, resp.Status(), resp.Data())
	pRef, err := data.FindProjectById("dimoxinil", false, false)
	s.NoError(err)
	s.Require().Equal("EVG", pRef.BuildBaronSettings.TicketCreateProject)
}

func (s *ProjectPatchByIDSuite) TestRunWithInvalidBbConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	jsonBody := []byte(`{"enabled": true, "build_baron_settings": {"ticket_create_project": "EVG"}}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(http.StatusBadRequest, resp.Status())
	errResp := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal("Must provide projects to search", errResp.Message)
}

func (s *ProjectPatchByIDSuite) TestGitTagVersionsEnabled() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	jsonBody := []byte(`{"enabled": true, "git_tag_versions_enabled": true, "aliases": [{"alias": "__git_tag", "git_tag": "my_git_tag", "variant": ".*", "task": ".*", "tag": ".*"}]}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
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

	jsonBody = []byte(`{"enabled": true, "git_tag_versions_enabled": true, "aliases": [{"alias": "__git_tag", "git_tag": "my_git_tag", "variant": ".*", "task": ".*", "tag": ".*"}]}`)
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err = s.rm.Parse(ctx, req)
	s.NoError(err)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(http.StatusOK, resp.Status())

	// verify that the repo fields weren't saved with the branch
	p, err := data.FindProjectById("dimoxinil", false, false)
	s.NoError(err)
	s.Require().NotNil(p)
	s.Empty(p.GitTagAuthorizedUsers)
	s.Nil(p.Restricted)
}

func (s *ProjectPatchByIDSuite) TestUpdateParsleyFilters() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})

	// fail - empty expression
	jsonBody := []byte(`{"parsley_filters": [{"expression": "", "case_sensitive": true, "exact_match": false}]}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp := s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(resp.Status(), http.StatusBadRequest)
	errResp := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(errResp.Message, "filter expression must be non-empty")

	// fail - invalid regular expression
	jsonBody = []byte(`{"parsley_filters": [{"expression": "*", "case_sensitive": true, "exact_match": false}]}`)
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err = s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(resp.Status(), http.StatusBadRequest)
	errResp = (resp.Data()).(gimlet.ErrorResponse)
	s.Contains(errResp.Message, "filter expression '*' is invalid regexp")

	// fail - duplicate filters
	jsonBody = []byte(`{"parsley_filters": [
		{"expression": "dupe", "case_sensitive": true, "exact_match": false}, 
		{"expression": "dupe", "case_sensitive": true, "exact_match": false},
		{"expression": "also_a_dupe", "case_sensitive": true, "exact_match": false},
		{"expression": "also_a_dupe", "case_sensitive": true, "exact_match": false}
	]}`)
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err = s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Require().Equal(resp.Status(), http.StatusBadRequest)
	errResp = (resp.Data()).(gimlet.ErrorResponse)
	s.Contains(errResp.Message, "duplicate filter with expression 'dupe'")
	s.Contains(errResp.Message, "duplicate filter with expression 'also_a_dupe'")

	// success
	jsonBody = []byte(`{"parsley_filters": [{"expression": "filter1", "case_sensitive": true, "exact_match": false}, {"expression": "filter2", "case_sensitive": true, "exact_match": false}]}`)
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	err = s.rm.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK, resp.Data())

	p, err := data.FindProjectById("dimoxinil", true, false)
	s.NoError(err)
	s.NotNil(p)
	s.Len(p.ParsleyFilters, 2)
}

func (s *ProjectPatchByIDSuite) TestPatchTriggerAliases() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	h := s.rm.(*projectIDPatchHandler)
	h.user = &user.DBUser{Id: "me"}

	jsonBody := []byte(`{"patch_trigger_aliases": [{"child_project_identifier": "child", "task_specifiers": [ {"task_regex": ".*", "variant_regex": ".*" }]}]}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
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

	p, err := data.FindProjectById("dimoxinil", true, false)
	s.NoError(err)
	s.False(p.PatchTriggerAliases == nil)
	s.Len(p.PatchTriggerAliases, 1)
	s.Equal(p.PatchTriggerAliases[0].ChildProject, "firstborn") // saves ID

	jsonBody = []byte(`{"patch_trigger_aliases": []}`) // empty list isn't nil
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.NoError(s.rm.Parse(ctx, req))
	s.NotNil(s.rm.(*projectIDPatchHandler).user)

	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err = data.FindProjectById("dimoxinil", true, false)
	s.NoError(err)
	s.NotNil(p.PatchTriggerAliases)
	s.Len(p.PatchTriggerAliases, 0)

	jsonBody = []byte(`{"patch_trigger_aliases": null}`)
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.NoError(s.rm.Parse(ctx, req))
	s.NotNil(s.rm.(*projectIDPatchHandler).user)
	resp = s.rm.Run(ctx)
	s.NotNil(resp)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	p, err = data.FindProjectById("dimoxinil", true, false)
	s.NoError(err)
	s.Nil(p.PatchTriggerAliases)
}

func (s *ProjectPatchByIDSuite) TestRotateAndDeleteProjectPodSecret() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "Test1"})
	h := s.rm.(*projectIDPatchHandler)
	h.user = &user.DBUser{Id: "me"}

	smClient, err := cloud.MakeSecretsManagerClient(ctx, s.env.Settings())
	s.Require().NoError(err)
	vault, err := cloud.MakeSecretsManagerVault(smClient)
	s.Require().NoError(err)

	cocoaMock.ResetGlobalSecretCache()
	defer cocoaMock.ResetGlobalSecretCache()

	// Create new pod secret.
	body := []byte(`{
	"container_secrets": [
		{
			"name": "super_secret",
			"type": "pod_secret",
			"should_rotate": true
		}
	]}`)
	req, err := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(body))
	s.Require().NoError(err)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.Require().NoError(s.rm.Parse(ctx, req))

	resp := s.rm.Run(ctx)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	dbProjRef, err := serviceModel.FindBranchProjectRef("dimoxinil")
	s.Require().NoError(err)
	s.Require().NotNil(dbProjRef)
	s.Require().Len(dbProjRef.ContainerSecrets, 1)
	s.Equal("super_secret", dbProjRef.ContainerSecrets[0].Name)
	s.EqualValues(serviceModel.ContainerSecretPodSecret, dbProjRef.ContainerSecrets[0].Type)
	s.NotZero(dbProjRef.ContainerSecrets[0].ExternalName)
	s.NotZero(dbProjRef.ContainerSecrets[0].ExternalID)

	externalID := dbProjRef.ContainerSecrets[0].ExternalID
	s.Require().NotNil(vault)
	initialStoredValue, err := vault.GetValue(ctx, externalID)
	s.Require().NoError(err)
	s.NotZero(initialStoredValue)

	// Rotate the existing pod secret's value.
	req, err = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(body))
	s.Require().NoError(err)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.Require().NoError(s.rm.Parse(ctx, req))

	resp = s.rm.Run(ctx)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	dbProjRef, err = serviceModel.FindBranchProjectRef("dimoxinil")
	s.Require().NoError(err)
	s.Require().NotNil(dbProjRef)
	s.Require().Len(dbProjRef.ContainerSecrets, 1)
	s.Equal("super_secret", dbProjRef.ContainerSecrets[0].Name)
	s.EqualValues(serviceModel.ContainerSecretPodSecret, dbProjRef.ContainerSecrets[0].Type)
	s.NotZero(dbProjRef.ContainerSecrets[0].ExternalName)
	s.NotZero(dbProjRef.ContainerSecrets[0].ExternalID)

	externalID = dbProjRef.ContainerSecrets[0].ExternalID
	s.Require().NotNil(vault)
	newStoredValue, err := vault.GetValue(ctx, externalID)
	s.Require().NoError(err)
	s.NotZero(newStoredValue)
	s.NotEqual(initialStoredValue, newStoredValue)

	// Delete the existing pod secret.
	body = []byte(`{
		"delete_container_secrets": ["super_secret"]
	}`)
	req, err = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/projects/dimoxinil", bytes.NewBuffer(body))
	s.Require().NoError(err)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "dimoxinil"})
	s.Require().NoError(s.rm.Parse(ctx, req))

	resp = s.rm.Run(ctx)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	dbProjRef, err = serviceModel.FindBranchProjectRef("dimoxinil")
	s.Require().NoError(err)
	s.Require().NotNil(dbProjRef)
	s.Empty(dbProjRef.ContainerSecrets, "container secret should have been deleted")

	_, err = vault.GetValue(ctx, externalID)
	s.Error(err, "secret should have been deleted from the vault")
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PUT /rest/v2/projects/{project_id}

type ProjectPutSuite struct {
	rm       gimlet.RouteHandler
	env      evergreen.Environment
	settings *evergreen.Settings

	suite.Suite
}

func TestProjectPutSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &ProjectPutSuite{
		env: testutil.NewEnvironment(ctx, t),
	}
	suite.Run(t, s)
}

func (s *ProjectPutSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(serviceModel.ProjectRefCollection, serviceModel.ProjectVarsCollection, fakeparameter.Collection, user.Collection))
	s.NoError(getTestProjectRef().Insert())

	settings := s.env.Settings()
	s.settings = settings
	settings.GithubOrgs = []string{"Rembrandt Q. Einstein"}
	s.NoError(evergreen.UpdateConfig(ctx, settings))

	s.rm = makePutProjectByID(s.env).(*projectIDPutHandler)
}

func (s *ProjectPutSuite) TestParse() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	req, _ := http.NewRequest(http.MethodPut, "http://example.com/api/rest/v2/projects/nutsandgum", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
}

func (s *ProjectPutSuite) TestRunNewWithValidEntity() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	u := user.DBUser{
		Id: "user",
	}
	s.NoError(u.Insert())
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
	h.project = model.APIProjectRef{
		Owner: utility.ToStringPtr("Rembrandt Q. Einstein"),
		Repo:  utility.ToStringPtr("nutsandgum"),
	}
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusCreated)

	p, err := data.FindProjectById("nutsandgum", false, false)
	s.NoError(err)
	s.Require().NotNil(p)
	s.NotEqual("nutsandgum", p.Id)
	s.Equal("nutsandgum", p.Identifier)
}

func (s *ProjectPutSuite) TestRunExistingFails() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	rm gimlet.RouteHandler

	suite.Suite
}

func TestProjectGetByIDSuite(t *testing.T) {
	suite.Run(t, new(ProjectGetByIDSuite))
}

func (s *ProjectGetByIDSuite) SetupTest() {
	s.NoError(db.ClearCollections(serviceModel.ProjectRefCollection, serviceModel.ProjectVarsCollection, fakeparameter.Collection, serviceModel.ProjectConfigCollection))
	s.NoError(getTestProjectRef().Insert())
	s.NoError(getTestVar().Insert())
	s.NoError(getTestProjectConfig().Insert())
	s.rm = makeGetProjectByID().(*projectIDGetHandler)
}

func (s *ProjectGetByIDSuite) TestRunNonExistingId() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := s.rm.(*projectIDGetHandler)
	h.projectName = "non-existent"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

func (s *ProjectGetByIDSuite) TestRunExistingId() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := s.rm.(*projectIDGetHandler)
	h.projectName = "dimoxinil"
	h.includeProjectConfig = true

	resp := s.rm.Run(ctx)
	s.Require().NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	projectRef, ok := resp.Data().(*model.APIProjectRef)
	s.Require().True(ok)
	s.Equal(utility.FromStringPtr(projectRef.Aliases[0].Alias), evergreen.CommitQueueAlias)
	cachedProject, err := data.FindProjectById(h.projectName, false, false)
	s.NoError(err)
	s.Equal(cachedProject.Repo, utility.FromStringPtr(projectRef.Repo))
	s.Equal(cachedProject.Owner, utility.FromStringPtr(projectRef.Owner))
	s.Equal(cachedProject.Branch, utility.FromStringPtr(projectRef.Branch))
	s.Equal(cachedProject.Enabled, utility.FromBoolPtr(projectRef.Enabled))
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
}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/projects

type ProjectGetSuite struct {
	route *projectGetHandler

	suite.Suite
}

func TestProjectGetSuite(t *testing.T) {
	suite.Run(t, new(ProjectGetSuite))
}

func (s *ProjectGetSuite) SetupSuite() {
	pRefs := []serviceModel.ProjectRef{
		{
			Id: "projectA",
		},
		{
			Id: "projectB",
		},
		{
			Id: "projectC",
		},
		{
			Id: "projectD",
		},
		{
			Id: "projectE",
		},
		{
			Id: "projectF",
		},
	}
	for _, pRef := range pRefs {
		s.NoError(pRef.Insert())
	}
}

func (s *ProjectGetSuite) SetupTest() {
	s.route = &projectGetHandler{
		url: "http://evergreen.example.net/",
	}
}

func (s *ProjectGetSuite) TestPaginatorShouldErrorIfNoResults() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.route.key = "zzz"
	s.route.limit = 1

	resp := s.route.Run(ctx)
	s.Equal(http.StatusNotFound, resp.Status())
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "no projects found")
}

func (s *ProjectGetSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.route.key = "projectC"
	s.route.limit = 1

	resp := s.route.Run(ctx)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.route.key = "projectA"
	s.route.limit = 100

	resp := s.route.Run(ctx)
	s.NotNil(resp)
	payload := resp.Data().([]interface{})

	s.Len(payload, 6)
	s.Equal(utility.ToStringPtr("projectA"), (payload[0]).(*model.APIProjectRef).Id, payload[0])
	s.Equal(utility.ToStringPtr("projectB"), (payload[1]).(*model.APIProjectRef).Id, payload[1])

	s.Nil(resp.Pages())
}

func (s *ProjectGetSuite) TestGetRecentVersions() {
	getVersions := makeFetchProjectVersionsLegacy()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// valid request with defaults
	request, err := http.NewRequest(http.MethodGet, "/projects/projectA/recent_versions", bytes.NewReader(nil))
	s.NoError(err)
	s.NoError(getVersions.Parse(ctx, request))

	// invalid limit
	request, err = http.NewRequest(http.MethodGet, "/projects/projectA/recent_versions?limit=asdf", bytes.NewReader(nil))
	s.NoError(err)
	err = getVersions.Parse(ctx, request)
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid limit")

	// invalid offset
	request, err = http.NewRequest(http.MethodGet, "/projects/projectA/recent_versions?offset=idk", bytes.NewReader(nil))
	s.NoError(err)
	err = getVersions.Parse(ctx, request)
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid offset")
}

func getTestVar() *serviceModel.ProjectVars {
	return &serviceModel.ProjectVars{
		Id:   "dimoxinil",
		Vars: map[string]string{"apple": "green", "banana": "yellow", "lemon": "yellow"},
	}
}

func getTestProjectConfig() *serviceModel.ProjectConfig {
	return &serviceModel.ProjectConfig{
		Id:      "dimoxinil",
		Project: "dimoxinil",
		ProjectConfigFields: serviceModel.ProjectConfigFields{
			CommitQueueAliases: []serviceModel.ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "dimoxinil",
					Alias:     evergreen.CommitQueueAlias,
				},
			},
		}}
}

func getTestAliases() []serviceModel.ProjectAlias {
	return []serviceModel.ProjectAlias{
		{
			ProjectID: "dimoxinil",
			Task:      ".*",
			Variant:   ".*",
			Alias:     evergreen.GithubPRAlias,
		},
		{
			ProjectID:   "dimoxinil",
			Task:        ".*",
			VariantTags: []string{"v1"},
			Alias:       evergreen.GitTagAlias,
		},
	}
}

func getTestProjectRef() *serviceModel.ProjectRef {
	return &serviceModel.ProjectRef{
		Owner:                 "dimoxinil",
		Repo:                  "dimoxinil-enterprise-repo",
		Branch:                "main",
		Enabled:               false,
		BatchTime:             0,
		RemotePath:            "evergreen.yml",
		Id:                    "dimoxinil",
		Identifier:            "dimoxinil",
		DisplayName:           "Dimoxinil",
		DeactivatePrevious:    utility.FalsePtr(),
		TracksPushEvents:      utility.FalsePtr(),
		PRTestingEnabled:      utility.FalsePtr(),
		VersionControlEnabled: utility.TruePtr(),
		CommitQueue: serviceModel.CommitQueueParams{
			Enabled: utility.FalsePtr(),
		},
		Hidden:                utility.FalsePtr(),
		PatchingDisabled:      utility.FalsePtr(),
		Admins:                []string{"langdon.alger"},
		NotifyOnBuildFailure:  utility.FalsePtr(),
		DisabledStatsCache:    utility.TruePtr(),
		ParameterStoreEnabled: true,
	}
}

func TestGetProjectTasks(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(db.ClearCollections(task.Collection, serviceModel.ProjectRefCollection, serviceModel.RepositoriesCollection))
	const projectId = "proj"
	project := serviceModel.ProjectRef{
		Id:         projectId,
		Identifier: "p1",
	}
	assert.NoError(project.Insert())
	assert.NoError(db.Insert(serviceModel.RepositoriesCollection, serviceModel.Repository{
		Project:             projectId,
		RevisionOrderNumber: 20,
	}))
	for i := 0; i <= 20; i++ {
		myTask := task.Task{
			Id:                  fmt.Sprintf("t%d", i),
			RevisionOrderNumber: 20 - i%2,
			DisplayName:         "t1",
			Project:             projectId,
			Status:              evergreen.TaskSucceeded,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(myTask.Insert())
	}

	h := getProjectTasksHandler{
		projectName: "p1",
		taskName:    "t1",
		opts: serviceModel.GetProjectTasksOpts{
			Limit: 10,
		},
	}

	resp := h.Run(ctx)
	assert.Equal(http.StatusOK, resp.Status())
	assert.Len(resp.Data(), 21)
}

func TestGetProjectVersions(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
		opts: serviceModel.GetVersionsOptions{
			Requester: evergreen.AdHocRequester,
			Limit:     20,
		},
	}
	resp := h.Run(ctx)
	respJson, err := json.Marshal(resp.Data())
	assert.NoError(err)
	assert.Contains(string(respJson), `"version_id":"v4"`)
	assert.NotContains(string(respJson), `"version_id":"v3"`)

	body := []byte(`{"revision_end": 1, "start": 4}`)
	url := "https://example.com/rest/v2/projects/something-else/versions"
	req, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(body))
	assert.NoError(err)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
	err = h.Parse(ctx, req)
	assert.NoError(err)
	assert.Contains(string(respJson), `"version_id":"v4"`)
	assert.Contains(string(respJson), `"version_id":"v1"`)
}

func TestDeleteProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(
		serviceModel.ProjectRefCollection,
		serviceModel.RepoRefCollection,
		fakeparameter.Collection,
		serviceModel.ProjectAliasCollection,
		serviceModel.ProjectVarsCollection,
		evergreen.ScopeCollection,
		user.Collection,
	))
	u := user.DBUser{
		Id: "me",
	}
	require.NoError(t, u.Insert())

	repo := serviceModel.RepoRef{
		ProjectRef: serviceModel.ProjectRef{
			Id:                    "repo_ref",
			Owner:                 "mongodb",
			Repo:                  "test_repo",
			ParameterStoreEnabled: true,
		},
	}
	assert.NoError(t, repo.Upsert())

	// Projects expected to be successfully deleted
	numGoodProjects := 2
	var projects []serviceModel.ProjectRef
	for i := 0; i < numGoodProjects; i++ {
		project := serviceModel.ProjectRef{
			Id:                    fmt.Sprintf("id_%d", i),
			Owner:                 "mongodb",
			Repo:                  "test_repo",
			Branch:                fmt.Sprintf("branch_%d", i),
			Enabled:               true,
			DisplayName:           fmt.Sprintf("display_%d", i),
			RepoRefId:             "repo_ref",
			TracksPushEvents:      utility.TruePtr(),
			PRTestingEnabled:      utility.TruePtr(),
			Admins:                []string{"admin0", "admin1"},
			NotifyOnBuildFailure:  utility.TruePtr(),
			ParameterStoreEnabled: true,
		}

		projects = append(projects, project)
		require.NoError(t, project.Add(&u))
		project.ParameterStoreVarsSynced = true
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

	pdh := projectDeleteHandler{}

	// Test cases:
	// 0) Project with 2 ProjectAliases and a ProjectVars
	// 1) Project with 0 ProjectAliases and no ProjectVars
	for i := 0; i < numGoodProjects; i++ {
		pdh.projectName = projects[i].Id
		resp := pdh.Run(ctx)
		assert.Equal(t, http.StatusOK, resp.Status())

		hiddenProj, err := serviceModel.FindMergedProjectRef(projects[i].Id, "", true)
		assert.NoError(t, err)
		skeletonProj := serviceModel.ProjectRef{
			Id:                       projects[i].Id,
			Owner:                    repo.Owner,
			Repo:                     repo.Repo,
			Branch:                   projects[i].Branch,
			RepoRefId:                repo.Id,
			Enabled:                  false,
			Hidden:                   utility.TruePtr(),
			ParameterStoreEnabled:    true,
			ParameterStoreVarsSynced: true,
		}
		assert.Equal(t, skeletonProj, *hiddenProj)

		projAliases, err := serviceModel.FindAliasesForProjectFromDb(projects[i].Id)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(projAliases))

		skeletonProjVars := serviceModel.ProjectVars{
			Id:   projects[i].Id,
			Vars: map[string]string{},
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
	nonTrackingProject := serviceModel.ProjectRef{
		Id:                    "non_tracking_project",
		RepoRefId:             "",
		ParameterStoreEnabled: true,
	}
	require.NoError(t, nonTrackingProject.Insert())
	pdh.projectName = nonTrackingProject.Id
	resp = pdh.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
}

func TestAttachProjectToRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(serviceModel.ProjectRefCollection,
		serviceModel.RepoRefCollection, serviceModel.ProjectVarsCollection, fakeparameter.Collection, user.Collection,
		evergreen.ScopeCollection, evergreen.RoleCollection, evergreen.ConfigCollection))
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	u := &user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert())

	pRef := serviceModel.ProjectRef{
		Id:                    "project1",
		Identifier:            "projectIdent",
		Owner:                 "evergreen-ci",
		Repo:                  "evergreen",
		Branch:                "main",
		RepoRefId:             "hello",
		Enabled:               true,
		Admins:                []string{"me"},
		ParameterStoreEnabled: true,
	}
	assert.NoError(t, pRef.Insert())
	projVars := serviceModel.ProjectVars{
		Id: "project1",
	}
	assert.NoError(t, projVars.Insert())

	req, _ := http.NewRequest(http.MethodPost, "http://example.com/api/rest/v2/projects/project1/attach_to_repo", nil)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "project1"})

	h := attachProjectToRepoHandler{}
	assert.Error(t, h.Parse(ctx, req)) // should fail because repoRefId is populated

	pRef.RepoRefId = ""
	assert.NoError(t, pRef.Upsert())
	assert.NoError(t, h.Parse(ctx, req))

	assert.NotNil(t, h.user)
	assert.NotNil(t, h.project)
	repoRef, err := serviceModel.FindRepoRefByOwnerAndRepo(h.project.Owner, h.project.Repo)
	assert.NoError(t, err)
	assert.Nil(t, repoRef) // repo ref doesn't exist before running

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Data())
	assert.Equal(t, resp.Status(), http.StatusOK)

	p, err := serviceModel.FindMergedProjectRef("projectIdent", "", false)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.True(t, p.UseRepoSettings())
	assert.NotEmpty(t, p.RepoRefId)
	assert.Contains(t, p.Admins, "me")

	u, err = user.FindOneById("me")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), serviceModel.GetRepoAdminRole(p.RepoRefId))
	hasPermission, err := serviceModel.UserHasRepoViewPermission(u, p.RepoRefId)
	assert.NoError(t, err)
	assert.True(t, hasPermission)

	repoRef, err = serviceModel.FindRepoRefByOwnerAndRepo(h.project.Owner, h.project.Repo)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)
}

func TestDetachProjectFromRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(serviceModel.ProjectRefCollection,
		serviceModel.RepoRefCollection, serviceModel.ProjectVarsCollection, user.Collection,
		evergreen.ScopeCollection, evergreen.RoleCollection))
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	u := &user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert())

	pRef := serviceModel.ProjectRef{
		Id:                    "project1",
		Identifier:            "projectIdent",
		Owner:                 "evergreen-ci",
		Repo:                  "evergreen",
		Branch:                "main",
		Enabled:               true,
		Admins:                []string{"me"},
		ParameterStoreEnabled: true,
	}
	assert.NoError(t, pRef.Insert())
	projVars := serviceModel.ProjectVars{
		Id: "project1",
	}
	assert.NoError(t, projVars.Insert())

	repoRef := &serviceModel.RepoRef{ProjectRef: serviceModel.ProjectRef{
		Id:                    "myRepo",
		Owner:                 "evergreen-ci",
		Repo:                  "evergreen",
		GitTagVersionsEnabled: utility.TruePtr(),
	}}
	assert.NoError(t, repoRef.Add(u))
	// assert that user _did_ have the right roles
	assert.Contains(t, u.Roles(), serviceModel.GetRepoAdminRole(repoRef.Id))

	req, _ := http.NewRequest(http.MethodPost, "http://example.com/api/rest/v2/projects/project1/detach_from_repo", nil)
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "project1"})

	h := detachProjectFromRepoHandler{}
	assert.Error(t, h.Parse(ctx, req)) // should fail because repoRefId isn't populated

	pRef.RepoRefId = repoRef.Id
	assert.NoError(t, pRef.Upsert())
	assert.NoError(t, h.Parse(ctx, req))

	assert.NotNil(t, h.user)
	assert.NotNil(t, h.project)

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Data())
	assert.Equal(t, resp.Status(), http.StatusOK)

	p, err := serviceModel.FindMergedProjectRef("projectIdent", "", false)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.False(t, p.UseRepoSettings())
	assert.Empty(t, p.RepoRefId)
	assert.Contains(t, p.Admins, "me")
	assert.True(t, p.IsGitTagVersionsEnabled()) // saved from the repo before detaching

	u, err = user.FindOneById("me")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.NotContains(t, u.Roles(), serviceModel.GetRepoAdminRole(p.RepoRefId))
}

func TestGetProjectTaskExecutions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, serviceModel.ProjectRefCollection))
	projRef := serviceModel.ProjectRef{
		Id:         "123",
		Identifier: "myProject",
	}
	assert.NoError(t, projRef.Insert())

	assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection))

	now := time.Now()
	earlier := time.Now().Add(-time.Hour)
	reallyEarly := now.Add(-12 * time.Hour)
	tasks := []task.Task{
		{
			Id:           "notFinished",
			Project:      "123",
			Status:       evergreen.TaskStarted,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			Execution:    1,
		},
		{
			Id:           "finished",
			Project:      "123",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   now,
			Execution:    1,
		},
		{
			Id:           "finishedEarlier",
			Project:      "123",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   earlier,
			Execution:    1,
		},
		{
			Id:           "patch",
			Project:      "123",
			Status:       evergreen.TaskSucceeded,
			Requester:    evergreen.PatchVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   now,
			Execution:    1,
		},
		{
			Id:           "tooEarly",
			Project:      "123",
			Status:       evergreen.TaskSucceeded,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   reallyEarly,
			Execution:    1,
		},
		{
			Id:           "wrongTask",
			Project:      "123",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task2",
			FinishTime:   now,
			Execution:    1,
		},
		{
			Id:           "wrongVariant",
			Project:      "123",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv2",
			DisplayName:  "task1",
			FinishTime:   now,
			Execution:    1,
		},
	}
	for _, each := range tasks {
		assert.NoError(t, each.Insert())
		each.Execution = 0
		// Duplicate everything for the old task collection to ensure this is working.
		assert.NoError(t, db.Insert(task.OldCollection, each))
	}
	for testName, test := range map[string]func(*testing.T, *getProjectTaskExecutionsHandler){
		"parseSuccess": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "requesters": ["gitter_request"],
                     "start_time": "2022-11-02T00:00:00.000Z",
                     "end_time": "2022-11-03T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)

			assert.NoError(t, err)
			assert.Equal(t, rm.projectId, "123")
			assert.Equal(t, rm.opts.TaskName, "t1")
			assert.Equal(t, rm.opts.BuildVariant, "bv1")
			assert.Equal(t, rm.opts.Requesters, []string{"gitter_request"})
			assert.Equal(t, rm.startTime, time.Date(2022, 11, 02, 0, 0, 0, 0, time.UTC))
			assert.Equal(t, rm.endTime, time.Date(2022, 11, 03, 0, 0, 0, 0, time.UTC))
		},
		"parseNoStartErrors": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "requesters": ["gitter_request"],
                     "end_time": "2022-11-03T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)
			assert.Error(t, err)
		},
		"parseNoEndSucceeds": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "requesters": ["gitter_request"],
                     "start_time": "2022-11-02T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)
			assert.NoError(t, err)
			assert.Equal(t, rm.projectId, "123")
			assert.Equal(t, rm.opts.TaskName, "t1")
			assert.Equal(t, rm.opts.BuildVariant, "bv1")
			assert.Equal(t, rm.opts.Requesters, []string{"gitter_request"})
			assert.Equal(t, rm.startTime, time.Date(2022, 11, 02, 0, 0, 0, 0, time.UTC))
			assert.True(t, utility.IsZeroTime(rm.endTime))
		},
		"parseNoRequesterSuccess": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "start_time": "2022-11-02T00:00:00.000Z",
                     "end_time": "2022-11-03T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/executions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)
			assert.NoError(t, err)
		},
		"parseInvalidRequester": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "requesters": ["what_am_i"],
                     "start_time": "2022-11-02T00:00:00.000Z",
                     "end_time": "2022-11-03T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/proj/num_excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "proj"})

			err := rm.Parse(context.Background(), req)
			assert.Error(t, err)
		},
		"parseInvalidTime": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "requesters": ["gitter_request"],
                     "start_time": "2022-11-02T00:00:00",
                     "end_time": "2022-11-03T00:00:00"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)
			assert.Error(t, err)
		},
		"parseInvalidTimeRange": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "build_variant": "bv1",
                     "requesters": ["gitter_request"],
                     "start_time": "2022-11-04T00:00:00.000Z",
                     "end_time": "2022-11-03T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)
			assert.Error(t, err)
		},
		"parseNoBuildVariant": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			body := []byte(
				`{
                     "task_name": "t1",
                     "requesters": ["gitter_request"],
                     "start_time": "2022-11-02T00:00:00.000Z",
                     "end_time": "2022-11-03T00:00:00.000Z"
                 }`)
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/rest/v2/projects/myProject/excutions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "myProject"})

			err := rm.Parse(context.Background(), req)
			assert.Error(t, err)
		},
		"successfulRun": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			rm.projectId = "123"
			rm.opts.BuildVariant = "bv1"
			rm.opts.TaskName = "task1"
			rm.startTime = now.Add(-20 * time.Hour)

			// Should include the finished tasks in both new and old.
			resp := rm.Run(context.Background())
			assert.NotNil(t, resp)
			assert.NotNil(t, resp.Data())
			respModel := resp.Data().(model.ProjectTaskExecutionResp)
			assert.Equal(t, 6, respModel.NumCompleted)
		},
		"emptyRun": func(t *testing.T, rm *getProjectTaskExecutionsHandler) {
			rm.projectId = "nothing"
			rm.opts.BuildVariant = "bv1"
			rm.opts.TaskName = "task1"
			rm.startTime = now.Add(-20 * time.Hour)

			resp := rm.Run(context.Background())
			assert.NotNil(t, resp)
			assert.NotNil(t, resp.Data())
			respModel := resp.Data().(model.ProjectTaskExecutionResp)
			assert.Equal(t, 0, respModel.NumCompleted)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			rm := makeGetProjectTaskExecutionsHandler().(*getProjectTaskExecutionsHandler)
			test(t, rm)
		})
	}
}

func TestModifyProjectVersions(t *testing.T) {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(serviceModel.ProjectRefCollection))
	const projectId = "proj"
	project := serviceModel.ProjectRef{
		Id: projectId,
	}
	assert.NoError(project.Insert())
	for testName, test := range map[string]func(*testing.T, *modifyProjectVersionsHandler){
		"parseSuccess": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"priority": -1,
	"revision_start": 4,
	"revision_end": 1
}
			`)
			req, _ := http.NewRequest(http.MethodPatch, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.NoError(err)
			assert.Equal(utility.FromInt64Ptr(rm.opts.Priority), evergreen.DisabledTaskPriority)
			assert.Equal(rm.opts.RevisionStart, 4)
			assert.Equal(rm.opts.RevisionEnd, 1)
		},
		"parseSuccessTimestamp": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"priority": -1,
	"start_time_str": "2022-11-02T00:00:00.000Z",
	"end_time_str": "2022-11-03T00:00:00.000Z"
}
			`)
			req, _ := http.NewRequest(http.MethodPatch, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.NoError(err)
			assert.Equal(utility.FromInt64Ptr(rm.opts.Priority), evergreen.DisabledTaskPriority)
			assert.Equal(rm.startTime, time.Date(2022, 11, 2, 0, 0, 0, 0, time.UTC))
			assert.Equal(rm.endTime, time.Date(2022, 11, 3, 0, 0, 0, 0, time.UTC))
		},
		"parseFaiWithNoPriority": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"revision_start": 4,
	"revision_end": 1
}
			`)
			req, _ := http.NewRequest(http.MethodPatch, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.Error(err)
		},
		"parseFaiWithInvalidStartAndEnd": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"priority": -1,
	"revision_start": 1,
	"revision_end": 4
}
			`)
			req, _ := http.NewRequest(http.MethodPatch, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.Error(err)
		},
		"parseFaiWithTimeStampAndOrder": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"priority": -1,
	"revision_start": 1,
	"revision_end": 4
	"start_time_str": "2022-11-02T00:00:00.000Z",
	"end_time_str": "2022-11-03T00:00:00.000Z"
}
			`)
			req, _ := http.NewRequest(http.MethodPatch, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.Error(err)
		},
		"parseFaiWithInvalidTimeStamp": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"priority": -1,
	"revision_start": 1,
	"revision_end": 4
	"start_time_str": "2022-11-03T00:00:00.000Z",
	"end_time_str": "2022-11-02T00:00:00.000Z"
}
			`)
			req, _ := http.NewRequest(http.MethodPatch, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.Error(err)
		},
		"parseFaiWithNoTimestampOrOrder": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			body := []byte(`
{
	"priority": -1,
}
			`)
			req, _ := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/projects/something-else/versions", bytes.NewBuffer(body))
			req = gimlet.SetURLVars(req, map[string]string{"project_id": projectId})
			err := rm.Parse(ctx, req)
			assert.Error(err)
		},
		"runSucceeds": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			rm.projectId = projectId
			rm.opts = serviceModel.ModifyVersionsOptions{
				Priority:      utility.ToInt64Ptr(evergreen.DisabledTaskPriority),
				RevisionStart: 4,
				RevisionEnd:   1,
				Requester:     evergreen.RepotrackerVersionRequester,
			}
			resp := rm.Run(ctx)
			assert.NotNil(resp)
			assert.Equal(http.StatusOK, resp.Status())
			foundTasks, err := task.FindWithFields(task.ByVersions([]string{"v1", "v2", "v3", "v4"}), task.IdKey, task.PriorityKey, task.ActivatedKey)
			assert.NoError(err)
			assert.Len(foundTasks, 4)
			var count int
			for _, tsk := range foundTasks {
				if tsk.Priority == evergreen.DisabledTaskPriority && !tsk.Activated {
					count++
				}
			}
			assert.Equal(4, count)
		},
		"runSucceedsTimeStamp": func(t *testing.T, rm *modifyProjectVersionsHandler) {
			rm.projectId = projectId
			rm.opts = serviceModel.ModifyVersionsOptions{
				Priority:  utility.ToInt64Ptr(evergreen.DisabledTaskPriority),
				Requester: evergreen.RepotrackerVersionRequester,
			}
			rm.startTime = time.Date(2022, 11, 2, 0, 0, 0, 0, time.UTC)
			rm.endTime = time.Date(2022, 11, 3, 0, 0, 0, 0, time.UTC)
			resp := rm.Run(ctx)
			assert.NotNil(resp)
			assert.Equal(http.StatusOK, resp.Status())
			foundTasks, err := task.FindWithFields(task.ByVersions([]string{"v1", "v2", "v3", "v4"}), task.IdKey, task.PriorityKey, task.ActivatedKey)
			assert.NoError(err)
			assert.Len(foundTasks, 4)
			var count int
			for _, tsk := range foundTasks {
				if tsk.Priority == evergreen.DisabledTaskPriority && !tsk.Activated {
					count++
				}
			}
			assert.Equal(2, count)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(db.ClearCollections(serviceModel.VersionCollection, task.Collection, build.Collection))
			v1 := serviceModel.Version{
				Id:                  "v1",
				Identifier:          projectId,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
				CreateTime:          time.Date(2022, time.November, 1, 0, 0, 0, 0, time.UTC),
			}
			assert.NoError(v1.Insert())
			v2 := serviceModel.Version{
				Id:                  "v2",
				Identifier:          projectId,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 2,
				CreateTime:          time.Date(2022, time.November, 2, 0, 0, 0, 0, time.UTC),
			}
			assert.NoError(v2.Insert())
			v3 := serviceModel.Version{
				Id:                  "v3",
				Identifier:          projectId,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 3,
				CreateTime:          time.Date(2022, time.November, 3, 0, 0, 0, 0, time.UTC),
			}
			assert.NoError(v3.Insert())
			v4 := serviceModel.Version{
				Id:                  "v4",
				Identifier:          projectId,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 4,
				CreateTime:          time.Date(2022, time.November, 4, 0, 0, 0, 0, time.UTC),
			}
			assert.NoError(v4.Insert())
			tasks := []task.Task{
				{
					Version:   "v1",
					BuildId:   "b1",
					Id:        "t1",
					Activated: true,
				},
				{
					Version:   "v2",
					BuildId:   "b2",
					Id:        "t2",
					Activated: true,
				},
				{
					Version:   "v3",
					BuildId:   "b3",
					Id:        "t3",
					Activated: true,
				},
				{
					Version:   "v4",
					BuildId:   "b4",
					Id:        "t4",
					Activated: true,
				},
			}
			builds := []build.Build{
				{
					Id:      "b1",
					Version: "v1",
				},
				{
					Id:      "b2",
					Version: "v2",
				},
				{
					Id:      "b3",
					Version: "v3",
				},
				{
					Id:      "b4",
					Version: "v4",
				},
			}
			for _, tsk := range tasks {
				assert.NoError(tsk.Insert())
			}
			for _, b := range builds {
				assert.NoError(b.Insert())
			}
			rm := makeModifyProjectVersionsHandler("").(*modifyProjectVersionsHandler)
			test(t, rm)
		})
	}
}
