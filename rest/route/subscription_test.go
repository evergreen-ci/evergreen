package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type SubscriptionRouteSuite struct {
	sc data.Connector
	suite.Suite
	postHandler MethodHandler
}

func TestSubscriptionRouteSuiteWithDB(t *testing.T) {
	s := new(SubscriptionRouteSuite)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.sc = &data.DBConnector{}

	suite.Run(t, s)
}

func (s *SubscriptionRouteSuite) SetupSuite() {
	// test getting the route handler
	const route = "/subscriptions"
	const version = 2
	routeManager := getSubscriptionRouteManager(route, version)
	s.NotNil(routeManager)
	s.Equal(route, routeManager.Route)
	s.Equal(version, routeManager.Version)
	s.postHandler = routeManager.Methods[0]
	s.IsType(&subscriptionPostHandler{}, s.postHandler.RequestHandler)
}

func (s *SubscriptionRouteSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection))
}

func (s *SubscriptionRouteSuite) TestSubscriptionPost() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"type":    "atype",
		"trigger": "atrigger",
		"owner":   "me",
		"selectors": []map[string]string{{
			"type": "seltype",
			"data": "seldata",
		}},
		"subscriber": map[string]string{
			"type":   "slack",
			"target": "slack message",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))

	// test creating a new subscription
	resp, err := s.postHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)

	dbSubscriptions, err := event.FindSubscriptionsByOwner("me")
	s.NoError(err)
	s.Len(dbSubscriptions, 1)
	s.Equal("atype", dbSubscriptions[0].Type)
	s.Equal("seldata", dbSubscriptions[0].Selectors[0].Data)
	s.Equal("slack", dbSubscriptions[0].Subscriber.Type)

	// test updating the same subscription
	id := dbSubscriptions[0].ID
	body = []map[string]interface{}{{
		"id":      id.Hex(),
		"type":    "new type",
		"trigger": "atrigger",
		"owner":   "me",
		"selectors": []map[string]string{{
			"type": "seltype",
			"data": "seldata",
		}},
		"subscriber": map[string]string{
			"type":   "slack",
			"target": "slack message",
		},
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))

	resp, err = s.postHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)

	dbSubscriptions, err = event.FindSubscriptionsByOwner("me")
	s.NoError(err)
	s.Len(dbSubscriptions, 1)
	s.Equal("new type", dbSubscriptions[0].Type)
}

func (s *SubscriptionRouteSuite) TestUnauthorizedUser() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"type":    "atype",
		"trigger": "atrigger",
		"owner":   "not_me",
		"selectors": []map[string]string{{
			"type": "seltype",
			"data": "seldata",
		}},
		"subscriber": map[string]string{
			"type":   "slack",
			"target": "slack message",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.EqualError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request), "Cannot change subscriptions for anyone other than yourself")
}
