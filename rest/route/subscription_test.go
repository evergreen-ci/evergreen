package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

type SubscriptionRouteSuite struct {
	sc data.Connector
	suite.Suite
	postHandler gimlet.RouteHandler
}

func TestSubscriptionRouteSuiteWithDB(t *testing.T) {
	s := new(SubscriptionRouteSuite)

	s.sc = &data.DBConnector{}

	suite.Run(t, s)
}

func (s *SubscriptionRouteSuite) SetupSuite() {
	s.postHandler = makeSetSubscrition(s.sc)
}

func (s *SubscriptionRouteSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection))
}

func (s *SubscriptionRouteSuite) TestSubscriptionPost() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
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
	s.NoError(s.postHandler.Parse(ctx, request))

	// test creating a new subscription
	resp := s.postHandler.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())

	s.NotNil(resp)

	dbSubscriptions, err := event.FindSubscriptionsByOwner("me", event.OwnerTypePerson)
	s.NoError(err)
	s.Require().Len(dbSubscriptions, 1)
	s.Equal(event.ResourceTypeTask, dbSubscriptions[0].ResourceType)
	s.Equal("seldata", dbSubscriptions[0].Selectors[0].Data)
	s.Equal("slack", dbSubscriptions[0].Subscriber.Type)

	// test updating the same subscription
	id := dbSubscriptions[0].ID
	body = []map[string]interface{}{{
		"id":            id,
		"resource_type": event.ResourceTypePatch,
		"trigger":       "outcome",
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
	s.NoError(s.postHandler.Parse(ctx, request))

	resp = s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	dbSubscriptions, err = event.FindSubscriptionsByOwner("me", event.OwnerTypePerson)
	s.NoError(err)
	s.Len(dbSubscriptions, 1)
	s.Equal(event.ResourceTypePatch, dbSubscriptions[0].ResourceType)
}

func (s *SubscriptionRouteSuite) TestProjectSubscription() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
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
	s.NoError(s.postHandler.Parse(ctx, request))

	// create a new subscription
	resp := s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	dbSubscriptions, err := event.FindSubscriptionsByOwner("myproj", event.OwnerTypeProject)
	s.NoError(err)
	s.Require().Len(dbSubscriptions, 1)
	s.Equal(event.ResourceTypeTask, dbSubscriptions[0].ResourceType)
	s.Equal("seldata", dbSubscriptions[0].Selectors[0].Data)
	s.Equal("email", dbSubscriptions[0].Subscriber.Type)

	// test updating the same subscription
	id := dbSubscriptions[0].ID
	body = []map[string]interface{}{{
		"id":            id,
		"resource_type": event.ResourceTypePatch,
		"trigger":       "outcome",
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
	s.NoError(s.postHandler.Parse(ctx, request))

	resp = s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	// get the updated subscription
	h := &subscriptionGetHandler{sc: s.sc}
	h.owner = "myproj"
	h.ownerType = string(event.OwnerTypeProject)
	resp = h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	sub := resp.Data().([]model.APISubscription)
	s.Equal(event.ResourceTypePatch, model.FromAPIString(sub[0].ResourceType))

	// delete the subscription
	d := &subscriptionDeleteHandler{sc: s.sc, id: id}
	resp = d.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	subscription, err := event.FindSubscriptionByID(id)
	s.NoError(err)
	s.Nil(subscription)
}

func (s *SubscriptionRouteSuite) TestPostUnauthorizedUser() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
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
	s.EqualError(s.postHandler.Parse(ctx, request), "401 (Unauthorized): Cannot change subscriptions for anyone other than yourself")
}

func (s *SubscriptionRouteSuite) TestGet() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	h := &subscriptionGetHandler{sc: s.sc}
	h.owner = "me"
	h.ownerType = string(event.OwnerTypePerson)
	resp := h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())

	s.TestSubscriptionPost()

	h.owner = "me"
	h.ownerType = string(event.OwnerTypePerson)
	resp = h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp.Data())
}

func (s *SubscriptionRouteSuite) TestDeleteValidation() {
	s.NoError(db.Clear(event.SubscriptionsCollection))
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "thanos"})
	d := &subscriptionDeleteHandler{}

	r, err := http.NewRequest(http.MethodDelete, "/subscriptions", nil)
	s.NoError(err)
	s.EqualError(d.Parse(ctx, r), "400 (Bad Request): Must specify an ID to delete")

	r, err = http.NewRequest(http.MethodDelete, "/subscriptions?id=5949645c9acd9704fdd202da", nil)
	s.NoError(err)
	s.EqualError(d.Parse(ctx, r), "404 (Not Found): Subscription not found")

	subscription := event.Subscription{
		ID:    "5949645c9acd9604fdd202da",
		Owner: "vision",
		Subscriber: event.Subscriber{
			Type: "email",
		},
	}
	s.NoError(subscription.Upsert())
	r, err = http.NewRequest(http.MethodDelete, "/subscriptions?id=5949645c9acd9604fdd202da", nil)
	s.NoError(err)
	s.EqualError(d.Parse(ctx, r), "401 (Unauthorized): Cannot delete subscriptions for someone other than yourself")
}

func (s *SubscriptionRouteSuite) TestGetWithoutUser() {
	s.PanicsWithValue("no user attached to request", func() {
		ctx := context.Background()
		h := &subscriptionGetHandler{}
		_ = h.Parse(ctx, nil)
	})
}

func (s *SubscriptionRouteSuite) TestDisallowedSubscription() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "object",
			"data": "version",
		}},
		"subscriber": map[string]interface{}{
			"type": "jira-issue",
			"target": map[string]string{
				"project":    "ABC",
				"issue_type": "Test",
			},
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Cannot notify by jira-issue for version")

	//test that project-level subscriptions are allowed
	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "project",
			"data": "mci",
		}},
		"subscriber": map[string]interface{}{
			"type": "jira-issue",
			"target": map[string]string{
				"project":    "ABC",
				"issue_type": "Test",
			},
		},
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))
}

func (s *SubscriptionRouteSuite) TestInvalidTriggerData() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
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
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Error validating subscription: foo must be a number")

	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
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
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Error validating subscription: -2 cannot be negative")

	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
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
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Error validating subscription: unable to parse a as float: strconv.ParseFloat: parsing \"a\": invalid syntax")

	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": "object",
			"data": "",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)

	request, err = http.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBuffer(jsonBody))
	s.NoError(err)
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Invalid selectors: Selector had empty type or data")
}

func (s *SubscriptionRouteSuite) TestInvalidRegexSelectors() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"regex_selectors": []map[string]string{{
			"type": "object",
			"data": "",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBuffer(jsonBody))
	s.NoError(err)
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Invalid regex selectors: Selector had empty type or data")

	body[0]["regex_selectors"] = []map[string]string{{
		"type": "",
		"data": "data",
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBuffer(jsonBody))
	s.NoError(err)
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Invalid regex selectors: Selector had empty type or data")
}

func (s *SubscriptionRouteSuite) TestRejectSubscriptionWithoutSelectors() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBuffer(jsonBody))
	s.NoError(err)
	s.EqualError(s.postHandler.Parse(ctx, request), "400 (Bad Request): Error validating subscription: must specify at least 1 selector")
}

func (s *SubscriptionRouteSuite) TestAcceptSubscriptionWithOnlyRegexSelectors() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	body := []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"regex_selectors": []map[string]string{{
			"type": "object",
			"data": "data",
		}},
		"subscriber": map[string]string{
			"type":   "email",
			"target": "yahoo@aol.com",
		},
	}}
	jsonBody, err := json.Marshal(body)
	s.NoError(err)
	request, err := http.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBuffer(jsonBody))
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))
}
