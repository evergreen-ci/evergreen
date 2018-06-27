package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
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

	dbSubscriptions, err := event.FindSubscriptionsByOwner("me", event.OwnerTypePerson)
	s.NoError(err)
	s.Require().Len(dbSubscriptions, 1)
	s.Equal("atype", dbSubscriptions[0].Type)
	s.Equal("seldata", dbSubscriptions[0].Selectors[0].Data)
	s.Equal("slack", dbSubscriptions[0].Subscriber.Type)

	// test updating the same subscription
	id := dbSubscriptions[0].ID
	body = []map[string]interface{}{{
		"id":            id.Hex(),
		"resource_type": "new type",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
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

	dbSubscriptions, err = event.FindSubscriptionsByOwner("me", event.OwnerTypePerson)
	s.NoError(err)
	s.Len(dbSubscriptions, 1)
	s.Equal("new type", dbSubscriptions[0].Type)
}

func (s *SubscriptionRouteSuite) TestProjectSubscription() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "myproj",
		"owner_type":    "project",
		"selectors": []map[string]string{{
			"type": "seltype",
			"data": "seldata",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "email message",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))

	// create a new subscription
	resp, err := s.postHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)

	dbSubscriptions, err := event.FindSubscriptionsByOwner("myproj", event.OwnerTypeProject)
	s.NoError(err)
	s.Require().Len(dbSubscriptions, 1)
	s.Equal("atype", dbSubscriptions[0].Type)
	s.Equal("seldata", dbSubscriptions[0].Selectors[0].Data)
	s.Equal("email", dbSubscriptions[0].Subscriber.Type)

	// test updating the same subscription
	id := dbSubscriptions[0].ID
	body = []map[string]interface{}{{
		"id":            id.Hex(),
		"resource_type": "new type",
		"trigger":       "atrigger",
		"owner":         "myproj",
		"owner_type":    "project",
		"selectors": []map[string]string{{
			"type": "seltype",
			"data": "seldata",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "email message",
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

	// get the updated subscription
	h := &subscriptionGetHandler{}
	h.owner = "myproj"
	h.ownerType = string(event.OwnerTypeProject)
	subs, err := h.Execute(ctx, s.sc)
	s.NoError(err)
	s.Require().Len(subs.Result, 1)
	sub := subs.Result[0].(*model.APISubscription)
	s.Equal("new type", model.FromAPIString(sub.ResourceType))

	// delete the subscription
	d := &subscriptionDeleteHandler{id: id.Hex()}
	_, err = d.Execute(ctx, s.sc)
	s.NoError(err)
	subscription, err := event.FindSubscriptionByID(id)
	s.NoError(err)
	s.Nil(subscription)
}

func (s *SubscriptionRouteSuite) TestPostUnauthorizedUser() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "not_me",
		"owner_type":    "person",
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

func (s *SubscriptionRouteSuite) TestGet() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	h := &subscriptionGetHandler{}
	h.owner = "me"
	h.ownerType = string(event.OwnerTypePerson)
	subs, err := h.Execute(ctx, s.sc)
	s.NoError(err)
	s.Len(subs.Result, 0)

	s.TestSubscriptionPost()

	h.owner = "me"
	h.ownerType = string(event.OwnerTypePerson)
	subs, err = h.Execute(ctx, s.sc)
	s.NoError(err)
	s.Len(subs.Result, 1)
}

func (s *SubscriptionRouteSuite) TestDeleteValidation() {
	s.NoError(db.Clear(event.SubscriptionsCollection))
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "thanos"})
	d := &subscriptionDeleteHandler{}

	r, err := http.NewRequest(http.MethodDelete, "/subscriptions", nil)
	s.NoError(err)
	s.EqualError(d.ParseAndValidate(ctx, r), "Must specify an ID to delete")

	r, err = http.NewRequest(http.MethodDelete, "/subscriptions?id=soul", nil)
	s.NoError(err)
	s.EqualError(d.ParseAndValidate(ctx, r), "soul is not a valid ObjectID")

	r, err = http.NewRequest(http.MethodDelete, "/subscriptions?id=5949645c9acd9704fdd202da", nil)
	s.NoError(err)
	s.EqualError(d.ParseAndValidate(ctx, r), "Subscription not found")

	subscription := event.Subscription{
		ID:    bson.ObjectIdHex("5949645c9acd9604fdd202da"),
		Owner: "vision",
		Subscriber: event.Subscriber{
			Type: "email",
		},
	}
	s.NoError(subscription.Upsert())
	r, err = http.NewRequest(http.MethodDelete, "/subscriptions?id=5949645c9acd9604fdd202da", nil)
	s.NoError(err)
	s.EqualError(d.ParseAndValidate(ctx, r), "Cannot delete subscriptions for someone other than yourself")
}

func (s *SubscriptionRouteSuite) TestGetWithoutUser() {
	s.PanicsWithValue("no user attached to request", func() {
		ctx := context.Background()
		h := &subscriptionGetHandler{}
		_ = h.ParseAndValidate(ctx, nil)
	})
}

func (s *SubscriptionRouteSuite) TestDisallowedSubscription() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "object",
			"data": "version",
		}},
		"subscriber": map[string]string{
			"type":   "jira-issue",
			"target": "ABC",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.EqualError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request), "Cannot notify by jira-issue for version")

	//test that project-level subscriptions are allowed
	body = []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "project",
			"data": "mci",
		}},
		"subscriber": map[string]string{
			"type":   "jira-issue",
			"target": "ABC",
		},
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))
}

func (s *SubscriptionRouteSuite) TestInvalidTriggerData() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "object",
			"data": "task",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
		"trigger_data": map[string]string{
			"task-duration-secs": "foo",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.EqualError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request), "Error validating subscription: foo must be a number")

	body = []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "object",
			"data": "task",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
		"trigger_data": map[string]string{
			"task-duration-secs": "-2",
		},
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.EqualError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request), "Error validating subscription: -2 cannot be negative")

	body = []map[string]interface{}{{
		"resource_type": "atype",
		"trigger":       "atrigger",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "object",
			"data": "task",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
		"trigger_data": map[string]string{
			"task-percent-change": "a",
		},
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.EqualError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request), "Error validating subscription: unable to parse a as float: strconv.ParseFloat: parsing \"a\": invalid syntax")
}
