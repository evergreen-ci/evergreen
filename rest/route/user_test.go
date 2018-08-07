package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
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
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.sc = &data.DBConnector{}

	suite.Run(t, s)
}

func (s *UserRouteSuite) SetupSuite() {
	s.postHandler = makeSetUserConfig(s.sc)
}

func (s *UserRouteSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection))
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
