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
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type SubscriptionRouteSuite struct {
	suite.Suite
	postHandler gimlet.RouteHandler
}

func TestSubscriptionRouteSuiteWithDB(t *testing.T) {
	s := new(SubscriptionRouteSuite)
	suite.Run(t, s)
}

func (s *SubscriptionRouteSuite) SetupSuite() {
	s.postHandler = makeSetSubscription()
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
			"type": event.SelectorObject,
			"data": "obj",
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
	s.Equal("obj", dbSubscriptions[0].Filter.Object)
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
			"type": event.SelectorObject,
			"data": "obj",
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
			"type": event.SelectorObject,
			"data": "obj",
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
	s.Equal("obj", dbSubscriptions[0].Filter.Object)
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
			"type": event.SelectorObject,
			"data": "obj",
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
	h := &subscriptionGetHandler{}
	h.owner = "myproj"
	h.ownerType = string(event.OwnerTypeProject)
	resp = h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	sub := resp.Data().([]model.APISubscription)
	s.Equal(event.ResourceTypePatch, utility.FromStringPtr(sub[0].ResourceType))

	// delete the subscription
	d := &subscriptionDeleteHandler{id: id}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: h.owner})
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
			"type": event.SelectorObject,
			"data": "obj",
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
	resp := s.postHandler.Run(ctx)
	s.Require().Equal(401, resp.Status())
	respErr, ok := resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal("cannot change subscriptions for anyone other than yourself", respErr.Message)
}

func (s *SubscriptionRouteSuite) TestGet() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	h := &subscriptionGetHandler{}
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
	err = d.Parse(ctx, r)
	s.Require().Error(err)
	s.Contains(err.Error(), "must specify a subscription ID to delete")

	r, err = http.NewRequest(http.MethodDelete, "/subscriptions?id=5949645c9acd9704fdd202da", nil)
	s.NoError(err)
	s.NoError(d.Parse(ctx, r))
	resp := d.Run(ctx)
	s.Equal(404, resp.Status())

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
	s.NoError(d.Parse(ctx, r))
	resp = d.Run(ctx)
	s.Equal(401, resp.Status())
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
			"type": event.SelectorObject,
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp := s.postHandler.Run(ctx)
	s.Require().Equal(400, resp.Status())
	respErr, ok := resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "cannot notify by subscriber type 'jira-issue' for selector 'version'")

	//test that project-level subscriptions are allowed
	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": event.SelectorProject,
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
			"type": event.SelectorObject,
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp := s.postHandler.Run(ctx)
	s.Require().Equal(400, resp.Status())
	respErr, ok := resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "invalid task duration")

	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": event.SelectorObject,
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp = s.postHandler.Run(ctx)
	s.Require().Equal(400, resp.Status())
	respErr, ok = resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "cannot be negative")

	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": event.SelectorObject,
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp = s.postHandler.Run(ctx)
	s.Require().Equal(400, resp.Status())
	respErr, ok = resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "parsing 'a' as float")

	body = []map[string]interface{}{{
		"resource_type": event.ResourceTypeTask,
		"trigger":       "outcome",
		"owner":         "me",
		"owner_type":    "person",
		"selectors": []map[string]string{{
			"type": event.SelectorObject,
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp = s.postHandler.Run(ctx)
	s.Require().Equal(400, resp.Status())
	respErr, ok = resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "selector 'object' has no data")
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
		"selectors": []map[string]string{{
			"type": event.SelectorID,
			"data": "abc123",
		}},
		"regex_selectors": []map[string]string{{
			"type": event.SelectorObject,
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp := s.postHandler.Run(ctx)
	s.Equal(400, resp.Status())
	respErr, ok := resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "selector 'object' has no data")

	body[0]["regex_selectors"] = []map[string]string{{
		"type": "",
		"data": "data",
	}}
	jsonBody, err = json.Marshal(body)
	s.NoError(err)
	request, err = http.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBuffer(jsonBody))
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))
	resp = s.postHandler.Run(ctx)
	s.Equal(400, resp.Status())
	respErr, ok = resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "selector has an empty type")
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
	s.NoError(s.postHandler.Parse(ctx, request))
	resp := s.postHandler.Run(ctx)
	s.Equal(400, resp.Status())
	respErr, ok := resp.Data().(gimlet.ErrorResponse)
	s.True(ok)
	s.Contains(respErr.Message, "no filter parameters specified")
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
			"type": event.SelectorObject,
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
