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
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UserRouteSuite struct {
	sc data.Connector
	suite.Suite
	postHandler gimlet.RouteHandler
}

func TestUserRouteSuiteWithDB(t *testing.T) {
	s := new(UserRouteSuite)

	s.sc = &data.DBConnector{}

	suite.Run(t, s)
}

func (s *UserRouteSuite) SetupSuite() {
	s.postHandler = makeSetUserConfig(s.sc)
}

func (s *UserRouteSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection, model.FeedbackCollection))
}

func (s *UserRouteSuite) TestUpdateNotifications() {
	_, err := user.GetOrCreateUser("me", "me", "foo@bar.com", "", "", nil)
	s.NoError(err)
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := map[string]interface{}{
		"slack_username": "@test",
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
}

func (s *UserRouteSuite) TestUndefinedInput() {
	_, err := user.GetOrCreateUser("me", "me", "foo@bar.com", "", "", nil)
	s.NoError(err)
	settings := user.UserSettings{
		SlackUsername: "something",
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
	s.EqualValues("you", dbUser.Settings.GithubUser.LastKnownAs)
}

func (s *UserRouteSuite) TestSaveFeedback() {
	_, err := user.GetOrCreateUser("me", "me", "foo@bar.com", "", "", nil)
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
	h gimlet.RouteHandler
	u user.DBUser
}

func TestPostUserPermissionSuite(t *testing.T) {
	suite.Run(t, &userPermissionPostSuite{})
}

func (s *userPermissionPostSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection}).Err()
	s.u = user.DBUser{
		Id: "user",
	}
	s.Require().NoError(s.u.Insert())
	s.h = makeModifyUserPermissions(&data.DBConnector{}, env.RoleManager())
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
	s.EqualError(s.h.Parse(context.Background(), request), "'' is not a valid resource_type")
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
	ctx := context.Background()
	env := evergreen.GetEnvironment()
	validBody := `{ "resource_type": "project", "resources": ["foo"], "permissions": {"project_tasks": 10} }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(validBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.NoError(s.h.Parse(ctx, request))
	resp := s.h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	roles, err := env.RoleManager().GetAllRoles()
	s.NoError(err)
	s.Len(roles, 1)
	dbUser, err := user.FindOneById(s.u.Id)
	s.NoError(err)
	s.Equal(dbUser.SystemRoles[0], roles[0].ID)
	foundScope, err := env.RoleManager().FindScopeForResources(evergreen.ProjectResourceType, "foo")
	s.NoError(err)
	s.NotNil(foundScope)

	// adjusting existing permissions should create a new role with the existing scope
	validBody = `{ "resource_type": "project", "resources": ["foo"], "permissions": {"project_tasks": 30} }`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(validBody)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": s.u.Id})
	s.NoError(err)
	s.NoError(s.h.Parse(ctx, request))
	_ = s.h.Run(ctx)
	roles, err = env.RoleManager().GetAllRoles()
	s.NoError(err)
	s.Len(roles, 2)
	newScope, err := env.RoleManager().FindScopeForResources(evergreen.ProjectResourceType, "foo")
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
	roles, err = env.RoleManager().GetAllRoles()
	s.NoError(err)
	s.Len(roles, 2)
	newScope, err = env.RoleManager().FindScopeForResources(evergreen.ProjectResourceType, "foo")
	s.NoError(err)
	s.NotNil(foundScope)
	s.Equal(newScope.ID, foundScope.ID)
	dbUser, err = user.FindOneById(s.u.Id)
	s.NoError(err)
	s.Len(dbUser.Roles(), 1)
}

func TestDeleteUserPermissions(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	env := evergreen.GetEnvironment()
	rm := env.RoleManager()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection}).Err()
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
	handler := userPermissionsDeleteHandler{sc: &data.DBConnector{}, rm: rm, userID: u.Id}
	ctx := context.Background()

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
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	env := evergreen.GetEnvironment()
	rm := env.RoleManager()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	u := user.DBUser{
		Id:          "user",
		SystemRoles: []string{"role1", evergreen.BasicProjectAccessRole, evergreen.BasicDistroAccessRole},
	}
	require.NoError(t, u.Insert())
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope1", Resources: []string{"resource1"}, Type: "project"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role1", Scope: "scope1", Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}}))
	handler := userPermissionsGetHandler{sc: &data.DBConnector{}, rm: rm, userID: u.Id}

	resp := handler.Run(context.Background())
	assert.Equal(t, http.StatusOK, resp.Status())
	data := resp.Data().([]rolemanager.PermissionSummary)
	assert.Len(t, data, 1)
	assert.Equal(t, "project", data[0].Type)
	assert.Equal(t, evergreen.ProjectSettingsEdit.Value, data[0].Permissions["resource1"][evergreen.PermissionProjectSettings])
}

func TestPostUserRoles(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.RoleCollection))
	rm := evergreen.GetEnvironment().RoleManager()
	u := user.DBUser{
		Id: "user",
	}
	require.NoError(t, u.Insert())
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role1", Scope: "scope1", Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}}))
	handler := userRolesPostHandler{sc: &data.DBConnector{}, rm: rm, userID: u.Id}
	ctx := context.Background()

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
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp = handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	dbUser, err = user.FindOneById(newId)
	assert.NoError(t, err)
	assert.Equal(t, []string{"role1"}, dbUser.Roles())
}

func TestServiceUserOperations(t *testing.T) {
	require.NoError(t, db.Clear(user.Collection))
	ctx := context.Background()

	body := `{ "user_id": "foo", "display_name": "service", "roles": ["one", "two"] }`
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	require.NoError(t, err)
	handler := makeUpdateServiceUser(&data.DBConnector{})
	assert.NoError(t, handler.Parse(ctx, request))
	_ = handler.Run(ctx)

	_, err = http.NewRequest(http.MethodGet, "", nil)
	require.NoError(t, err)
	handler = makeGetServiceUsers(&data.DBConnector{})
	resp := handler.Run(ctx)
	users, valid := resp.Data().([]restModel.APIDBUser)
	assert.True(t, valid)
	assert.Len(t, users, 1)
	assert.Equal(t, "foo", *users[0].UserID)
	assert.Equal(t, "service", *users[0].DisplayName)
	assert.Equal(t, []string{"one", "two"}, users[0].Roles)
	assert.True(t, users[0].OnlyApi)

	body = `{ "user_id": "foo", "display_name": "different" }`
	request, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer([]byte(body)))
	require.NoError(t, err)
	handler = makeUpdateServiceUser(&data.DBConnector{})
	assert.NoError(t, handler.Parse(ctx, request))
	_ = handler.Run(ctx)

	_, err = http.NewRequest(http.MethodGet, "", nil)
	require.NoError(t, err)
	handler = makeGetServiceUsers(&data.DBConnector{})
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
	handler = makeDeleteServiceUser(&data.DBConnector{})
	assert.NoError(t, handler.Parse(ctx, request))
	_ = handler.Run(ctx)

	request, err = http.NewRequest(http.MethodGet, "", nil)
	require.NoError(t, err)
	handler = makeGetServiceUsers(&data.DBConnector{})
	resp = handler.Run(ctx)
	users, valid = resp.Data().([]restModel.APIDBUser)
	assert.True(t, valid)
	assert.Len(t, users, 0)
}

func TestGetUsersForRole(t *testing.T) {
	require.NoError(t, db.Clear(user.Collection))
	ctx := context.Background()

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
	handler := makeGetUsersWithRole(&data.DBConnector{})
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

func TestGetUsersForResourceId(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	rm := evergreen.GetEnvironment().RoleManager()

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
		Id: "nobody",
		SystemRoles: []string{
			"other_project_access",
		},
	}
	assert.NoError(t, db.InsertMany(user.Collection, u1, u2, u3))
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
	assert.NoError(t, rm.UpdateRole(basicRole))
	assert.NoError(t, rm.UpdateRole(superRole))
	assert.NoError(t, rm.UpdateRole(otherRole))
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
			handler := makeGetAllUsersPermissions(&data.DBConnector{}, rm)
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
			handler := makeGetAllUsersPermissions(&data.DBConnector{}, rm)
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
			handler := makeGetAllUsersPermissions(&data.DBConnector{}, rm)
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
