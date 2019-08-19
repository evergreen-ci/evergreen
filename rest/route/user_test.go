package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
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
	_, err := model.GetOrCreateUser("me", "me", "foo@bar.com")
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
	_, err := model.GetOrCreateUser("me", "me", "foo@bar.com")
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

	_, err := model.GetOrCreateUser("john.smith", "John Smith", "john@smith.com")
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
	s.Equal(dbUser.DisplayName(), restModel.FromAPIString(authorInfo.DisplayName))
	s.Equal(dbUser.Email(), restModel.FromAPIString(authorInfo.Email))
}

func (s *UserRouteSuite) TestSaveFeedback() {
	_, err := model.GetOrCreateUser("me", "me", "foo@bar.com")
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

func TestRoleRoutes(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(user.RoleCollection))

	ctx := context.Background()
	body := map[string]interface{}{}
	jsonBody, err := json.Marshal(body)
	assert.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/roles", buffer)
	assert.NoError(err)
	postHandler := makeUpdateRoleHandler(&data.DBConnector{})
	assert.EqualError(postHandler.Parse(ctx, request), "role ID must be set")

	// test inserting a role
	body = map[string]interface{}{
		"id":          "myRole",
		"permissions": map[string]string{"p1": "true"},
		"owners":      []string{"me"},
	}
	jsonBody, err = json.Marshal(body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/roles", buffer)
	assert.NoError(err)
	postHandler = makeUpdateRoleHandler(&data.DBConnector{})
	assert.NoError(postHandler.Parse(ctx, request))
	resp := postHandler.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	// test reading that role
	getHandler := makeGetAllRolesHandler(&data.DBConnector{})
	resp = getHandler.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	roles, valid := resp.Data().([]restModel.APIRole)
	assert.True(valid)
	assert.Equal(body["id"], restModel.FromAPIString(roles[0].Id))
	assert.Equal(body["permissions"], roles[0].Permissions)
	assert.Equal(body["owners"], roles[0].Owners)

	// updating a role
	body = map[string]interface{}{
		"id":          "myRole",
		"permissions": map[string]string{"p2": "false"},
		"owners":      []string{"me"},
	}
	jsonBody, err = json.Marshal(body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/roles", buffer)
	assert.NoError(err)
	assert.NoError(postHandler.Parse(ctx, request))
	resp = postHandler.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	resp = getHandler.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	roles, valid = resp.Data().([]restModel.APIRole)
	assert.True(valid)
	assert.Equal(body["id"], restModel.FromAPIString(roles[0].Id))
	assert.Equal(body["permissions"], roles[0].Permissions)
}
