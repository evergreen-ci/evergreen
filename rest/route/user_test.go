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
	_, err := model.GetOrCreateUser("me", "me", "foo@bar.com", "", "")
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
	_, err := model.GetOrCreateUser("me", "me", "foo@bar.com", "", "")
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

func (s *UserRouteSuite) TestUserAuthorInfo() {
	route := makeFetchUserAuthor(s.sc)
	authorInfoHandler, ok := route.(*userAuthorGetHandler)
	s.True(ok)
	authorInfoHandler.userID = "john.smith"

	_, err := model.GetOrCreateUser("john.smith", "John Smith", "john@smith.com", "", "")
	s.NoError(err)

	ctx := context.Background()
	resp := authorInfoHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	data := resp.Data()
	authorInfo, ok := data.(restModel.APIUserAuthorInformation)
	s.True(ok)

	dbUser, err := user.FindOne(user.ById("john.smith"))
	s.NoError(err)
	s.Equal(dbUser.DisplayName(), restModel.FromStringPtr(authorInfo.DisplayName))
	s.Equal(dbUser.Email(), restModel.FromStringPtr(authorInfo.Email))
}

func (s *UserRouteSuite) TestSaveFeedback() {
	_, err := model.GetOrCreateUser("me", "me", "foo@bar.com", "", "")
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
