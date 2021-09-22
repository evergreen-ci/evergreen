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
		SystemRoles: []string{"role1", "role2", "role3"},
	}
	require.NoError(t, u.Insert())
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope1", Resources: []string{"resource1"}, Type: "project"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope2", Resources: []string{"resource2"}, Type: "project"}))
	require.NoError(t, rm.AddScope(gimlet.Scope{ID: "scope3", Resources: []string{"resource3"}, Type: "distro"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role1", Scope: "scope1"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role2", Scope: "scope2"}))
	require.NoError(t, rm.UpdateRole(gimlet.Role{ID: "role3", Scope: "scope3"}))
	handler := userPermissionsDeleteHandler{sc: &data.DBConnector{}, rm: rm, userID: u.Id}
	ctx := context.Background()

	body := `{ "resource_type": "project" }`
	request, err := http.NewRequest(http.MethodDelete, "", bytes.NewBuffer([]byte(body)))
	request = gimlet.SetURLVars(request, map[string]string{"user_id": u.Id})
	require.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())
	dbUser, err := user.FindOneById(u.Id)
	require.NoError(t, err)
	assert.Len(t, dbUser.SystemRoles, 1)
	assert.Equal(t, "role3", dbUser.SystemRoles[0])

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
		SystemRoles: []string{"role1"},
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
