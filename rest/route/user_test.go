package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UserRouteSuite struct {
	suite.Suite
	postHandler gimlet.RouteHandler
}

func TestUserRouteSuiteWithDB(t *testing.T) {
	s := new(UserRouteSuite)
	suite.Run(t, s)
}

func (s *UserRouteSuite) SetupSuite() {
	s.postHandler = makeSetUserConfig()
}

func (s *UserRouteSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection, model.FeedbackCollection))
}

func (s *UserRouteSuite) TestUpdateNotifications() {
	_, err := user.GetOrCreateUser("me", "me", "", "token", "", nil)
	s.NoError(err)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := map[string]interface{}{
		"slack_username":  "@test",
		"slack_member_id": "NOTES25BA",
		"notifications": map[string]string{
			"build_break":  "slack",
			"patch_finish": "email",
		},
	}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/users/settings", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))

	resp := s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	dbUser, err := user.FindOne(user.ById("me"))
	s.NoError(err)
	s.EqualValues(user.PreferenceSlack, dbUser.Settings.Notifications.BuildBreak)
	s.EqualValues(user.PreferenceEmail, dbUser.Settings.Notifications.PatchFinish)
	s.EqualValues("test", dbUser.Settings.SlackUsername)
	s.EqualValues("NOTES25BA", dbUser.Settings.SlackMemberId)
}

func (s *UserRouteSuite) TestUndefinedInput() {
	_, err := user.GetOrCreateUser("me", "me", "", "token", "", nil)
	s.NoError(err)
	settings := user.UserSettings{
		SlackUsername: "something",
		SlackMemberId: "NOTES25BA",
		GithubUser: user.GithubUser{
			LastKnownAs: "you",
		},
	}
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me", Settings: settings})
	body := map[string]interface{}{
		"notifications": map[string]string{
			"build_break": "slack",
		},
	}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/users/settings", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))

	resp := s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	dbUser, err := user.FindOne(user.ById("me"))
	s.NoError(err)
	s.EqualValues(user.PreferenceSlack, dbUser.Settings.Notifications.BuildBreak)
	s.EqualValues("something", dbUser.Settings.SlackUsername)
	s.EqualValues("NOTES25BA", dbUser.Settings.SlackMemberId)
	s.EqualValues("you", dbUser.Settings.GithubUser.LastKnownAs)
}

func (s *UserRouteSuite) TestSaveFeedback() {
	_, err := user.GetOrCreateUser("me", "me", "", "token", "", nil)
	s.NoError(err)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := map[string]interface{}{
		"spruce_feedback": map[string]interface{}{
			"type": "someType",
			"questions": []map[string]interface{}{
				{"id": "1", "prompt": "this is a question", "answer": "this is an answer"},
			},
		},
	}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/users/settings", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))

	resp := s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	feedback, err := model.FindFeedbackOfType("someType")
	s.NoError(err)
	s.Len(feedback, 1)
	s.Equal("me", feedback[0].User)
	s.NotEqual(time.Time{}, feedback[0].SubmittedAt)
	s.Len(feedback[0].Questions, 1)
}

type userPermissionPostSuite struct {
	suite.Suite
	h   gimlet.RouteHandler
	u   user.DBUser
	env evergreen.Environment
}

func TestPostUserPermissionSuite(t *testing.T) {
	s := &userPermissionPostSuite{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.env = testutil.NewEnvironment(ctx, t)
	suite.Run(t, s)
}

func (s *userPermissionPostSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	s.Require().NoError(db.CreateCollections(evergreen.ScopeCollection))
	s.u = user.DBUser{
		Id: "user",
	}
	s.Require().NoError(s.u.Insert())
	s.h = makeModifyUserPermissions(s.env.RoleManager())
}

func (s *userPermissionPostSuite) TestNoUser() {
	invalidBody := `{ "foo": "bar" }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(invalidBody)))
	s.NoError(err)
	s.EqualError(s.h.Parse(context.Background(), request), "no user found")
}

func (s *userPermissionPostSuite) TestNoResourceType() {
	invalidBody := `{ "foo": "bar" }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(invalidBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.EqualError(s.h.Parse(context.Background(), request), "invalid resource type ''")
}

func (s *userPermissionPostSuite) TestNoResource() {
	invalidBody := `{ "resource_type": "project" }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(invalidBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.EqualError(s.h.Parse(context.Background(), request), "resources cannot be empty")
}

func (s *userPermissionPostSuite) TestInvalidPermissions() {
	ctx := context.Background()
	invalidBody := `{ "resource_type": "project", "resources": ["foo"], "permissions": {"asdf": 10} }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(invalidBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.NoError(s.h.Parse(ctx, request))
	resp := s.h.Run(ctx)
	s.EqualValues("'asdf' is not a valid permission", resp.Data())
}

func (s *userPermissionPostSuite) TestValidInput() {
	// valid input that should create a new role + scope
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	validBody := `{ "resource_type": "project", "resources": ["foo"], "permissions": {"project_tasks": 10} }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(validBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.NoError(s.h.Parse(ctx, request))
	resp := s.h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	roles, err := s.env.RoleManager().GetAllRoles()
	s.NoError(err)
	s.Len(roles, 1)
	dbUser, err := user.FindOneById(s.u.Id)
	s.NoError(err)
	s.Equal(dbUser.SystemRoles[0], roles[0].ID)
	foundScope, err := s.env.RoleManager().FindScopeForResources(evergreen.ProjectResourceType, "foo")
	s.NoError(err)
	s.NotNil(foundScope)

	// adjusting existing permissions should create a new role with the existing scope
	validBody = `{ "resource_type": "project", "resources": ["foo"], "permissions": {"project_tasks": 30} }`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(validBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.NoError(s.h.Parse(ctx, request))
	_ = s.h.Run(ctx)
	roles, err = s.env.RoleManager().GetAllRoles()
	s.NoError(err)
	s.Len(roles, 2)
	newScope, err := s.env.RoleManager().FindScopeForResources(evergreen.ProjectResourceType, "foo")
	s.NoError(err)
	s.NotNil(foundScope)
	s.Equal(newScope.ID, foundScope.ID)

	// a matching role should just be added
	dbUser, err = user.FindOneById(s.u.Id)
	s.NoError(err)
	for _, role := range dbUser.Roles() {
		s.NoError(dbUser.RemoveRole(role))
	}
	_ = s.h.Run(ctx)
	roles, err = s.env.RoleManager().GetAllRoles()
	s.NoError(err)
	s.Len(roles, 2)
	newScope, err = s.env.RoleManager().FindScopeForResources(evergreen.ProjectResourceType, "foo")
	s.NoError(err)
	s.NotNil(foundScope)
	s.Equal(newScope.ID, foundScope.ID)
	dbUser, err = user.FindOneById(s.u.Id)
	s.NoError(err)
	s.Len(dbUser.Roles(), 1)
}

func TestProjectSettingsUpdateViewRepo(t *testing.T) {
	assert.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection,
		model.ProjectRefCollection, model.RepoRefCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()

	// Setup project and repo data to ensure that repo roles are added.
	pRef := model.ProjectRef{
		Id:        "attached",
		RepoRefId: "myRepo",
		Enabled:   true,
	}
	assert.NoError(t, pRef.Insert())
	pRef = model.ProjectRef{
		Id:      "unattached",
		Enabled: true,
	}
	assert.NoError(t, pRef.Insert())

	repoRef := model.RepoRef{model.ProjectRef{
		Id: "myRepo",
	}}
	assert.NoError(t, repoRef.Upsert())
	scope := gimlet.Scope{
		ID:        "myRepo_scope",
		Resources: []string{"myRepo"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(t, rm.AddScope(scope))
	newViewRole := gimlet.Role{
		ID:    model.GetViewRepoRole("myRepo"),
		Scope: scope.ID,
		Permissions: gimlet.Permissions{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
		},
	}
	assert.NoError(t, rm.UpdateRole(newViewRole))

	u := user.DBUser{
		Id:          "me",
		SystemRoles: []string{},
	}
	require.NoError(t, u.Insert())

	// We should add the repo view role for the attached project, and not error because of the unattached project.
	handler := userPermissionsPostHandler{rm: rm, userID: u.Id}
	validBody := `{ "resource_type": "project", "resources": ["attached", "unattached"], "permissions": {"project_settings": 20} }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(validBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": "me"})
	assert.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	roles, err := rm.GetAllRoles()
	assert.NoError(t, err)
	assert.Len(t, roles, 2)
	dbUser, err := user.FindOneById(u.Id)
	assert.NoError(t, err)
	require.Len(t, dbUser.SystemRoles, 2)
	assert.Contains(t, dbUser.SystemRoles, roles[0].ID)
	assert.Contains(t, dbUser.SystemRoles, roles[1].ID)
	assert.Contains(t, dbUser.SystemRoles, model.GetViewRepoRole("myRepo"))
}

func TestDeleteUserPermissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	rm := env.RoleManager()
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	u := user.DBUser{
		Id:          "user",
		SystemRoles: []string{"role1", "role2", "role3", evergreen.BasicProjectAccessRole},
	}
	require.NoError(t, u.Insert())
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope1", Resources: []string{"resource1"}, Type: "project"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope2", Resources: []string{"resource2"}, Type: "project"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope3", Resources: []string{"resource3"}, Type: "distro"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: evergreen.AllProjectsScope, Resources: []string{"resource1", "resource2"}, Type: "project"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role1", Scope: "scope1"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role2", Scope: "scope2"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role3", Scope: "scope3"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: evergreen.BasicProjectAccessRole, Scope: evergreen.AllProjectsScope}))
	handler := userPermissionsDeleteHandler{rm: rm, userID: u.Id}

	body := `{ "resource_type": "project", "resource_id": "resource1" }`
	request, err := http.NewRequest(http.MethodDelete, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": u.Id})
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	dbUser, err := user.FindOneById(u.Id)
	require.NoError(t, err)
	assert.Len(t, dbUser.SystemRoles, 3)
	assert.NotContains(t, dbUser.SystemRoles, "role1")
	assert.Contains(t, dbUser.SystemRoles, evergreen.BasicProjectAccessRole)

	body = `{ "resource_type": "all" }`
	request, err = http.NewRequest(http.MethodDelete, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": u.Id})
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp = handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	dbUser, err = user.FindOneById(u.Id)
	require.NoError(t, err)
	assert.Len(t, dbUser.SystemRoles, 0)
}

func TestGetUserPermissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection, model.ProjectRefCollection))
	rm := env.RoleManager()
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	u := user.DBUser{
		Id:          "user",
		SystemRoles: []string{"role1", "role2", "role3", evergreen.BasicProjectAccessRole, evergreen.BasicDistroAccessRole},
	}
	projectRefs := []model.ProjectRef{
		{
			Id: "resource1",
		},
		{
			Id:     "resource2",
			Hidden: utility.TruePtr(),
		},
		{
			Id: "resource3",
		},
	}
	for _, projectRef := range projectRefs {
		require.NoError(t, projectRef.Upsert())
	}
	require.NoError(t, u.Insert())
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope1", Resources: []string{"resource1"}, Type: "project"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope2", Resources: []string{"resource2"}, Type: "project"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope3", Resources: []string{"resource3"}, Type: "project"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role1", Scope: "scope1", Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role2", Scope: "scope2", Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role3", Scope: "scope3", Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}}))

	handler := userPermissionsGetHandler{rm: rm, userID: u.Id}

	resp := handler.Run(context.Background())
	assert.Equal(t, http.StatusOK, resp.Status())
	data := resp.Data().([]rolemanager.PermissionSummary)
	assert.Len(t, data, 1)
	assert.Equal(t, "project", data[0].Type)
	require.Len(t, data[0].Permissions, 2)
	assert.Equal(t, evergreen.ProjectSettingsEdit.Value, data[0].Permissions["resource1"][evergreen.PermissionProjectSettings])
	assert.Nil(t, data[0].Permissions["resource2"])
	assert.Equal(t, evergreen.ProjectSettingsEdit.Value, data[0].Permissions["resource3"][evergreen.PermissionProjectSettings])
}

func TestPostUserRoles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	env.SetUserManager(serviceutil.MockUserManager{})
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.RoleCollection))
	rm := env.RoleManager()
	u := user.DBUser{
		Id: "user",
	}
	require.NoError(t, u.Insert())
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role1", Scope: "scope1", Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}}))
	handler := userRolesPostHandler{rm: rm, userID: u.Id}

	body := `{ "foo": "bar" }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": u.Id})
	require.NoError(t, err)
	assert.Error(t, handler.Parse(ctx, request))

	body = `{"roles": ["notarole"]}`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": u.Id})
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, http.StatusNotFound, resp.Status())

	body = `{"roles": ["role1"]}`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": u.Id})
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp = handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	dbUser, err := user.FindOneById(u.Id)
	assert.NoError(t, err)
	assert.Equal(t, []string{"role1"}, dbUser.Roles())

	const newId = "user2"
	body = `{"create_user": true, "roles": ["role1"]}`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": newId})
	u = user.DBUser{
		Id: newId,
	}
	require.NoError(t, u.Insert())
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp = handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	dbUser, err = user.FindOneById(newId)
	assert.NoError(t, err)
	assert.Equal(t, []string{"role1"}, dbUser.Roles())
}

func TestServiceUserOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Clear(user.Collection))

	body := `{ "user_id": "foo", "display_name": "service", "roles": ["one", "two"], "email_address":"myemail@mailplace.com" }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	require.NoError(t, err)
	handler := makeUpdateServiceUser()
	assert.NoError(t, handler.Parse(ctx, request))
	_ = handler.Run(ctx)

	_, err = http.NewRequest(http.MethodGet, "", nil)
	require.NoError(t, err)
	handler = makeGetServiceUsers()
	resp := handler.Run(ctx)
	users, valid := resp.Data().([]restModel.APIDBUser)
	assert.True(t, valid)
	assert.Len(t, users, 1)
	assert.Equal(t, "foo", *users[0].UserID)
	assert.Equal(t, "service", *users[0].DisplayName)
	assert.Equal(t, []string{"one", "two"}, users[0].Roles)
	assert.Equal(t, "myemail@mailplace.com", *users[0].EmailAddress)
	assert.True(t, users[0].OnlyApi)

	body = `{ "user_id": "foo", "display_name": "different" }`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	require.NoError(t, err)
	handler = makeUpdateServiceUser()
	assert.NoError(t, handler.Parse(ctx, request))
	_ = handler.Run(ctx)

	_, err = http.NewRequest(http.MethodGet, "", nil)
	require.NoError(t, err)
	handler = makeGetServiceUsers()
	resp = handler.Run(ctx)
	users, valid = resp.Data().([]restModel.APIDBUser)
	assert.True(t, valid)
	assert.Len(t, users, 1)
	assert.Equal(t, "foo", *users[0].UserID)
	assert.Equal(t, "different", *users[0].DisplayName)

	request, err = http.NewRequest(http.MethodDelete, "", nil)
	require.NoError(t, err)
	assert.NoError(t, request.ParseForm())
	request.Form.Add("id", "foo")
	handler = makeDeleteServiceUser()
	assert.NoError(t, handler.Parse(ctx, request))
	_ = handler.Run(ctx)

	request, err = http.NewRequest(http.MethodGet, "", nil)
	require.NoError(t, err)
	handler = makeGetServiceUsers()
	resp = handler.Run(ctx)
	users, valid = resp.Data().([]restModel.APIDBUser)
	assert.True(t, valid)
	assert.Len(t, users, 0)
}

func TestGetUsersForRole(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Clear(user.Collection))

	u1 := user.DBUser{
		Id: "me",
		SystemRoles: []string{
			"admin_access",
			"basic_project_access",
			"something_else",
		},
	}
	u2 := user.DBUser{
		Id: "mini-me",
		SystemRoles: []string{
			"basic_project_access",
		},
	}
	u3 := user.DBUser{
		Id: "you",
		SystemRoles: []string{
			"nothing_useful",
		},
	}
	assert.NoError(t, db.InsertMany(user.Collection, u1, u2, u3))

	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/roles/basic_project_access/users", nil)
	require.NoError(t, err)
	req = gimlet.SetURLVars(req, map[string]string{"role_id": "basic_project_access"})
	handler := makeGetUsersWithRole()
	assert.NoError(t, handler.Parse(ctx, req))
	assert.Equal(t, handler.(*usersWithRoleGetHandler).role, "basic_project_access")
	resp := handler.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status(), http.StatusOK)
	usersWithRole, ok := resp.Data().(*UsersWithRoleResponse)
	assert.True(t, ok)
	assert.NotNil(t, usersWithRole.Users)
	assert.Len(t, usersWithRole.Users, 2)
	assert.Contains(t, utility.FromStringPtr(usersWithRole.Users[0]), u1.Id)
	assert.Contains(t, utility.FromStringPtr(usersWithRole.Users[1]), u2.Id)
}

func TestRemoveHiddenProjects(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	projectRefs := []model.ProjectRef{
		{
			Id:     "project1",
			Hidden: utility.TruePtr(),
		},
		{
			Id: "project2",
		},
		{
			Id:     "project3",
			Hidden: utility.TruePtr(),
		},
		{
			Id: "project4",
		},
	}
	for _, projectRef := range projectRefs {
		require.NoError(t, projectRef.Upsert())
	}

	permissions := []rolemanager.PermissionSummary{
		{
			Type: evergreen.ProjectResourceType,
			Permissions: rolemanager.PermissionsForResources{
				"project1": gimlet.Permissions{
					"project_settings": 10,
				},
				"project2": gimlet.Permissions{
					"project_settings": 10,
				},
				"project3": gimlet.Permissions{
					"project_settings": 10,
				},
				"project4": gimlet.Permissions{
					"project_settings": 10,
				},
			},
		},
		{
			Type: evergreen.DistroResourceType,
			Permissions: rolemanager.PermissionsForResources{
				"distro1": gimlet.Permissions{
					"distro_settings": 10,
				},
			},
		},
	}

	assert.NoError(t, removeHiddenProjects(permissions))
	require.Len(t, permissions, 2)
	require.Len(t, permissions[0].Permissions, 2)
	require.Nil(t, permissions[0].Permissions["project1"])
	require.NotNil(t, permissions[0].Permissions["project2"])
	require.Nil(t, permissions[0].Permissions["project3"])
	require.NotNil(t, permissions[0].Permissions["project4"])
}

func TestGetUsersForResourceId(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	rm := env.RoleManager()

	u1 := user.DBUser{
		Id: "me",
		SystemRoles: []string{
			"basic_project_access",
			"super_project_access",
		},
	}
	u2 := user.DBUser{
		Id: "you",
		SystemRoles: []string{
			"basic_project_access",
			"other_project_access",
		},
	}
	u3 := user.DBUser{
		Id:      "nobody",
		OnlyAPI: false,
		SystemRoles: []string{
			"other_project_access",
		},
	}
	u4 := user.DBUser{ // should never be returned
		Id:      "Data",
		OnlyAPI: true,
		SystemRoles: []string{
			"bot_access",
		},
	}
	assert.NoError(t, db.InsertMany(user.Collection, u1, u2, u3, u4))
	basicRole := gimlet.Role{
		ID:    "basic_project_access",
		Scope: "some_projects",
		Permissions: gimlet.Permissions{
			evergreen.PermissionTasks:       evergreen.TasksView.Value,
			evergreen.PermissionAnnotations: evergreen.AnnotationsView.Value,
			evergreen.PermissionLogs:        evergreen.LogsView.Value,
		},
	}
	superRole := gimlet.Role{
		ID:    "super_project_access",
		Scope: "all_projects",
		Permissions: gimlet.Permissions{
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionAnnotations:     evergreen.AnnotationsModify.Value,
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		},
	}
	otherRole := gimlet.Role{
		ID:    "other_project_access",
		Scope: "one_project",
		Permissions: gimlet.Permissions{
			evergreen.PermissionTasks: evergreen.TasksView.Value,
		},
	}
	botRole := gimlet.Role{
		ID:    "bot_access",
		Scope: "all_projects",
		Permissions: gimlet.Permissions{
			evergreen.PermissionPatches: evergreen.PatchSubmitAdmin.Value,
		},
	}
	assert.NoError(t, rm.UpdateRole(basicRole))
	assert.NoError(t, rm.UpdateRole(superRole))
	assert.NoError(t, rm.UpdateRole(otherRole))
	assert.NoError(t, rm.UpdateRole(botRole))
	someScope := gimlet.Scope{
		ID:        "some_projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"p1", "p2"},
	}
	superScope := gimlet.Scope{
		ID:        "all_projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"p1", "p2", "p3"},
	}
	oneScope := gimlet.Scope{
		ID:        "one_project",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"p2"},
	}

	assert.NoError(t, rm.AddScope(someScope))
	assert.NoError(t, rm.AddScope(superScope))
	assert.NoError(t, rm.AddScope(oneScope))

	for testName, testCase := range map[string]func(t *testing.T){
		"p1": func(t *testing.T) {
			body := []byte(`{"resource_type": "project", "resource_id":"p1"}`)
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/permissions", bytes.NewBuffer(body))
			require.NoError(t, err)
			handler := makeGetAllUsersPermissions(rm)
			assert.NoError(t, handler.Parse(context.TODO(), req))
			assert.Equal(t, handler.(*allUsersPermissionsGetHandler).input.ResourceId, "p1")
			assert.Equal(t, handler.(*allUsersPermissionsGetHandler).input.ResourceType, evergreen.ProjectResourceType)
			resp := handler.Run(context.TODO())
			assert.Equal(t, resp.Status(), http.StatusOK)
			userPermissions, ok := resp.Data().(UsersPermissionsResult)
			assert.True(t, ok)
			assert.Len(t, userPermissions, 1) // only user1 should be included; user2 only has basic access
			u1Permissions := userPermissions[u1.Username()]
			assert.Equal(t, u1Permissions[evergreen.PermissionTasks], evergreen.TasksAdmin.Value)
			assert.Equal(t, u1Permissions[evergreen.PermissionAnnotations], evergreen.AnnotationsModify.Value)
			assert.Equal(t, u1Permissions[evergreen.PermissionLogs], 0) // only relevant to basic project access
			assert.Equal(t, u1Permissions[evergreen.PermissionProjectSettings], evergreen.ProjectSettingsEdit.Value)
		},
		"p2": func(t *testing.T) {
			body := []byte(`{"resource_type": "project", "resource_id":"p2"}`)
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/permissions", bytes.NewBuffer(body))
			require.NoError(t, err)
			handler := makeGetAllUsersPermissions(rm)
			assert.NoError(t, handler.Parse(context.TODO(), req))
			assert.Equal(t, handler.(*allUsersPermissionsGetHandler).input.ResourceId, "p2")
			assert.Equal(t, handler.(*allUsersPermissionsGetHandler).input.ResourceType, evergreen.ProjectResourceType)
			resp := handler.Run(context.TODO())
			assert.Equal(t, resp.Status(), http.StatusOK)
			userPermissions, ok := resp.Data().(UsersPermissionsResult)
			assert.True(t, ok)
			assert.Len(t, userPermissions, 3)
			u1Permissions := userPermissions[u1.Username()]
			assert.Equal(t, u1Permissions[evergreen.PermissionTasks], evergreen.TasksAdmin.Value)
			assert.Equal(t, u1Permissions[evergreen.PermissionAnnotations], evergreen.AnnotationsModify.Value)
			assert.Equal(t, u1Permissions[evergreen.PermissionLogs], 0) // only relevant to basic project access
			assert.Equal(t, u1Permissions[evergreen.PermissionProjectSettings], evergreen.ProjectSettingsEdit.Value)
			u2Permissions := userPermissions[u2.Username()]
			assert.Equal(t, u2Permissions[evergreen.PermissionTasks], evergreen.TasksView.Value)
			assert.Equal(t, u2Permissions[evergreen.PermissionLogs], 0)            // only relevant to basic project access
			assert.Equal(t, u2Permissions[evergreen.PermissionProjectSettings], 0) // wasn't given admin
			u3Permissions := userPermissions[u3.Username()]
			assert.Len(t, u3Permissions, 1)
			assert.Equal(t, u3Permissions[evergreen.PermissionTasks], evergreen.TasksView.Value)
		},
		"p3": func(t *testing.T) {
			body := []byte(`{"resource_type": "project", "resource_id":"p3"}`)
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/permissions", bytes.NewBuffer(body))
			require.NoError(t, err)
			handler := makeGetAllUsersPermissions(rm)
			assert.NoError(t, handler.Parse(context.TODO(), req))
			assert.Equal(t, handler.(*allUsersPermissionsGetHandler).input.ResourceId, "p3")
			assert.Equal(t, handler.(*allUsersPermissionsGetHandler).input.ResourceType, evergreen.ProjectResourceType)
			resp := handler.Run(context.TODO())
			assert.Equal(t, resp.Status(), http.StatusOK)
			userPermissions, ok := resp.Data().(UsersPermissionsResult)
			assert.True(t, ok)
			assert.Len(t, userPermissions, 1) // only user1 has permissions for p3
			u1Permissions := userPermissions[u1.Username()]
			assert.Len(t, u1Permissions, 3)
			assert.Equal(t, u1Permissions[evergreen.PermissionTasks], evergreen.TasksAdmin.Value)
			assert.Equal(t, u1Permissions[evergreen.PermissionAnnotations], evergreen.AnnotationsModify.Value)
			assert.Equal(t, u1Permissions[evergreen.PermissionProjectSettings], evergreen.ProjectSettingsEdit.Value)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}

func TestRenameUser(t *testing.T) {
	body := []byte(`{"email": "me@awesome.com", "new_email":"new_me@still_awesome.com"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	for testName, testCase := range map[string]func(t *testing.T){
		"UserAlreadyExists": func(t *testing.T) {
			// Insert additional testing to cover the case of the user already being created.
			pNew := patch.Patch{
				Id:          mgobson.NewObjectId(),
				Author:      "new_me",
				PatchNumber: 1,
			}
			assert.NoError(t, pNew.Insert())
			newUsr := user.DBUser{
				Id:           "new_me",
				EmailAddress: "new_me@still_awesome.com",
				APIKey:       "my_original_key",
				PatchNumber:  1,
			}
			assert.NoError(t, newUsr.Insert())
			req, err := http.NewRequest(http.MethodPost, "http://example.com/api/rest/v2/users/rename_user", bytes.NewBuffer(body))
			require.NoError(t, err)
			handler := makeRenameUser(env)
			assert.NoError(t, handler.Parse(ctx, req))
			assert.Equal(t, handler.(*renameUserHandler).newEmail, "new_me@still_awesome.com")
			require.NotNil(t, handler.(*renameUserHandler).oldUsr)
			assert.Equal(t, handler.(*renameUserHandler).oldUsr.Id, "me")
			resp := handler.Run(ctx)
			assert.Equal(t, resp.Status(), http.StatusOK)

			newUsrFromDb, err := user.FindOneById("new_me")
			assert.NoError(t, err)
			assert.NotNil(t, newUsrFromDb)
			assert.NotEqual(t, newUsr.APIKey, newUsrFromDb.GetAPIKey())
			assert.Equal(t, "new_me@still_awesome.com", newUsrFromDb.Email())
			assert.Equal(t, newUsrFromDb.PatchNumber, 8)
			assert.Equal(t, 12, newUsrFromDb.Settings.GithubUser.UID)

			hosts, err := host.Find(ctx, host.ByUserWithUnterminatedStatus("new_me"))
			assert.NoError(t, err)
			assert.Len(t, hosts, 1)

			volumes, err := host.FindVolumesByUser("new_me")
			assert.NoError(t, err)
			assert.Len(t, volumes, 1)

			patches, err := patch.Find(db.Query(mgobson.M{patch.AuthorKey: "new_me"}))
			assert.NoError(t, err)
			assert.Len(t, patches, 3)
			for _, p := range patches {
				// Verify the newest patch had the number updated.
				if p.Id == pNew.Id {
					assert.Equal(t, p.PatchNumber, 8)
				}
			}
		},
		"UserDoesntAlreadyExist": func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://example.com/api/rest/v2/users/rename_user", bytes.NewBuffer(body))
			require.NoError(t, err)
			handler := makeRenameUser(env)
			assert.NoError(t, handler.Parse(ctx, req))
			assert.Equal(t, handler.(*renameUserHandler).newEmail, "new_me@still_awesome.com")
			require.NotNil(t, handler.(*renameUserHandler).oldUsr)
			assert.Equal(t, handler.(*renameUserHandler).oldUsr.Id, "me")
			resp := handler.Run(ctx)
			assert.Equal(t, resp.Status(), http.StatusOK)

			newUsrFromDb, err := user.FindOneById("new_me")
			assert.NoError(t, err)
			assert.NotNil(t, newUsrFromDb)
			assert.NotEmpty(t, newUsrFromDb.GetAPIKey())
			assert.Equal(t, "new_me@still_awesome.com", newUsrFromDb.Email())
			assert.Equal(t, newUsrFromDb.PatchNumber, 7)
			assert.Equal(t, 12, newUsrFromDb.Settings.GithubUser.UID)

			hosts, err := host.Find(ctx, host.ByUserWithUnterminatedStatus("new_me"))
			assert.NoError(t, err)
			assert.Len(t, hosts, 1)

			volumes, err := host.FindVolumesByUser("new_me")
			assert.NoError(t, err)
			assert.Len(t, volumes, 1)

			patches, err := patch.Find(db.Query(mgobson.M{patch.AuthorKey: "new_me"}))
			assert.NoError(t, err)
			assert.Len(t, patches, 2)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(user.Collection, host.Collection, host.VolumesCollection, patch.Collection))

			// Replicate the shape of the index on the DB.
			require.NoError(t, modelutil.AddTestIndexes(user.Collection, true, true,
				bsonutil.GetDottedKeyName(user.SettingsKey, user.UserSettingsGithubUserKey, user.GithubUserUIDKey)))

			h1 := host.Host{
				Id:        "h1",
				StartedBy: "me",
				UserHost:  true,
				Status:    evergreen.HostTerminated,
			}
			h2 := host.Host{
				Id:        "h2",
				StartedBy: "me",
				UserHost:  true,
				Status:    evergreen.HostRunning,
			}
			h3 := host.Host{
				Id:        "h3",
				StartedBy: "you",
				UserHost:  true,
				Status:    evergreen.HostRunning,
			}
			assert.NoError(t, db.InsertMany(host.Collection, h1, h2, h3))

			v1 := host.Volume{
				ID:        "v1",
				CreatedBy: "me",
			}
			v2 := host.Volume{
				ID:        "v2",
				CreatedBy: "you",
			}
			assert.NoError(t, db.InsertMany(host.VolumesCollection, v1, v2))

			p1 := patch.Patch{
				Id:          mgobson.NewObjectId(),
				Author:      "me",
				PatchNumber: 6,
			}
			p2 := patch.Patch{
				Id:          mgobson.NewObjectId(),
				Author:      "me",
				PatchNumber: 7,
			}
			assert.NoError(t, db.InsertMany(patch.Collection, p1, p2))

			someOtherUser := user.DBUser{
				Id:           "some_other_me",
				EmailAddress: "me@awesome.com",
				APIKey:       "my_key",
				PatchNumber:  7,
				Settings: user.UserSettings{GithubUser: user.GithubUser{
					UID: 0, // Verify there's no issue inserting an empty UID if there's another empty UID
				}},
			}
			oldUsr := user.DBUser{
				Id:           "me",
				EmailAddress: "me@awesome.com",
				APIKey:       "my_key",
				PatchNumber:  7,
				Settings: user.UserSettings{GithubUser: user.GithubUser{
					UID: 12,
				}},
			}
			assert.NoError(t, someOtherUser.Insert())
			assert.NoError(t, oldUsr.Insert())
			testCase(t)
		})
	}
}

func TestOffboardUserHandlerHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, user.Collection))
	h0 := host.Host{
		Id:           "h0",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostTerminated,
		CreationTime: time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC),
		Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
	}
	h1 := host.Host{
		Id:           "h1",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostRunning,
		CreationTime: time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC),
		NoExpiration: true,
		Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
	}
	h2 := host.Host{
		Id:           "h2",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostRunning,
		CreationTime: time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC),
		Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock},
	}
	h3 := host.Host{
		Id:           "h3",
		StartedBy:    "user0",
		UserHost:     true,
		Status:       evergreen.HostTerminated,
		CreationTime: time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC),
		Distro:       distro.Distro{Id: "ubuntu-1804", Provider: evergreen.ProviderNameMock},
	}
	v1 := host.Volume{
		ID:           "v1",
		CreatedBy:    "user0",
		NoExpiration: true,
	}
	assert.NoError(t, h0.Insert(ctx))
	assert.NoError(t, h1.Insert(ctx))
	assert.NoError(t, h2.Insert(ctx))
	assert.NoError(t, h3.Insert(ctx))
	assert.NoError(t, v1.Insert())

	handler := offboardUserHandler{}
	json := []byte(`{"email": "user0@mongodb.com"}`)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "root"})
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/users/offboard_user?dry_run=true", bytes.NewBuffer(json))
	assert.Error(t, handler.Parse(ctx, req)) // user not inserted

	u := user.DBUser{Id: "user0"}
	assert.NoError(t, u.Insert())
	req, _ = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/users/offboard_user?dry_run=true", bytes.NewBuffer(json))
	assert.NoError(t, handler.Parse(ctx, req))
	assert.Equal(t, "user0", handler.user)
	assert.True(t, handler.dryRun)

	resp := handler.Run(ctx)
	require.Equal(t, http.StatusOK, resp.Status())
	res, ok := resp.Data().(restModel.APIOffboardUserResults)
	assert.True(t, ok)
	require.Len(t, res.TerminatedHosts, 1)
	assert.Equal(t, "h1", res.TerminatedHosts[0])
	require.Len(t, res.TerminatedVolumes, 1)
	assert.Equal(t, "v1", res.TerminatedVolumes[0])
	hostFromDB, err := host.FindOneByIdOrTag(ctx, h1.Id)
	assert.NoError(t, err)
	assert.NotNil(t, hostFromDB)
	assert.True(t, hostFromDB.NoExpiration)
}

func TestOffboardUserHandlerAdminis(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, model.ProjectRefCollection))
	projectRef0 := &model.ProjectRef{
		Owner:     "mongodb",
		Repo:      "test_repo0",
		Branch:    "main",
		Enabled:   true,
		BatchTime: 10,
		Id:        "test0",
		Admins:    []string{"user1", "user0"},
	}
	projectRef1 := &model.ProjectRef{
		Owner:     "mongodb",
		Repo:      "test_repo1",
		Branch:    "main",
		Enabled:   true,
		BatchTime: 10,
		Id:        "test1",
		Admins:    []string{"user1", "user2"},
	}

	assert.NoError(t, projectRef0.Insert())
	assert.NoError(t, projectRef1.Insert())

	offboardedUser := "user0"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	userManager := env.UserManager()

	handler := offboardUserHandler{
		dryRun: true,
		env:    env,
		user:   offboardedUser,
	}
	resp := handler.Run(gimlet.AttachUser(ctx, &user.DBUser{Id: "root"}))
	require.Equal(t, http.StatusOK, resp.Status())
	assert.Contains(t, projectRef0.Admins, offboardedUser)

	handler.dryRun = false
	handler.env.SetUserManager(serviceutil.MockUserManager{})
	resp = handler.Run(gimlet.AttachUser(ctx, &user.DBUser{Id: "root"}))
	require.Equal(t, http.StatusOK, resp.Status())
	env.SetUserManager(userManager)

	projectRefs, err := model.FindAllMergedProjectRefs()
	assert.NoError(t, err)
	require.Len(t, projectRefs, 2)
	for _, projRef := range projectRefs {
		assert.NotContains(t, projRef.Admins, offboardedUser)
	}
}

func TestGetUserHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	usrToRetrieve := user.DBUser{
		Id:           "beep.boop",
		DispName:     "robots are good",
		EmailAddress: "bots_r@us.com",
		APIKey:       "secret_key",
		PatchNumber:  12,
		OnlyAPI:      true,
		SystemRoles: []string{
			"bot_access",
		},
	}
	me := user.DBUser{
		Id:           "me",
		EmailAddress: "i@rock.com",
		APIKey:       "my_key",
	}

	for testName, testCase := range map[string]func(t *testing.T){
		"UserNotFound": func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/no_one", nil)
			req = gimlet.SetURLVars(req, map[string]string{"user_id": "no_one"})

			require.NoError(t, err)
			handler := makeGetUserHandler()

			assert.NoError(t, handler.Parse(ctx, req))
			userHandler, ok := handler.(*getUserHandler)
			require.True(t, ok)
			assert.Equal(t, userHandler.userId, "no_one")

			resp := handler.Run(gimlet.AttachUser(ctx, &me))
			assert.Equal(t, resp.Status(), http.StatusNotFound)
		}, "UserFound": func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://example.com/api/rest/v2/users/beep.boop", nil)
			req = gimlet.SetURLVars(req, map[string]string{"user_id": "beep.boop"})

			require.NoError(t, err)
			handler := makeGetUserHandler()

			assert.NoError(t, handler.Parse(ctx, req))
			userHandler, ok := handler.(*getUserHandler)
			require.True(t, ok)
			assert.Equal(t, userHandler.userId, "beep.boop")

			resp := handler.Run(gimlet.AttachUser(ctx, &me))
			assert.Equal(t, resp.Status(), http.StatusOK)
			respUsr, ok := resp.Data().(*restModel.APIDBUser)
			require.True(t, ok)
			assert.NotEmpty(t, respUsr)
			assert.Equal(t, usrToRetrieve.Id, utility.FromStringPtr(respUsr.UserID))
			assert.Equal(t, usrToRetrieve.DisplayName(), utility.FromStringPtr(respUsr.DisplayName))
			assert.Equal(t, usrToRetrieve.EmailAddress, utility.FromStringPtr(respUsr.EmailAddress))
			assert.Equal(t, usrToRetrieve.OnlyAPI, respUsr.OnlyApi)
			assert.Equal(t, usrToRetrieve.Roles(), respUsr.Roles)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(user.Collection))
			assert.NoError(t, usrToRetrieve.Insert())
			assert.NoError(t, me.Insert())
			testCase(t)
		})
	}

}
