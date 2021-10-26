package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestGetSubscriptions(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(event.SubscriptionsCollection))

	subs := []event.Subscription{
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "someone",
			OwnerType:    event.OwnerTypePerson,
			ResourceType: "PATCH",
			Trigger:      "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "someoneelse",
			OwnerType:    event.OwnerTypePerson,
			ResourceType: "PATCH",
			Trigger:      "outcomeelse",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
	}

	for i := range subs {
		assert.NoError(subs[i].Upsert())
	}

	c := &DBSubscriptionConnector{}
	apiSubs, err := c.GetSubscriptions("someone", event.OwnerTypePerson)
	assert.NoError(err)
	assert.Len(apiSubs, 1)

	apiSubs, err = c.GetSubscriptions("someoneelse", event.OwnerTypePerson)
	assert.NoError(err)
	assert.Len(apiSubs, 1)

	apiSubs, err = c.GetSubscriptions("who", event.OwnerTypePerson)
	assert.NoError(err)
	assert.Len(apiSubs, 0)

	apiSubs, err = c.GetSubscriptions("", event.OwnerTypePerson)
	assert.EqualError(err, "400 (Bad Request): no subscription owner provided")
	assert.Len(apiSubs, 0)
}

func TestSaveProjectSubscriptions(t *testing.T) {
	c := &DBSubscriptionConnector{}
	for name, test := range map[string]func(t *testing.T, subs []restModel.APISubscription){
		"InvalidSubscription": func(t *testing.T, subs []restModel.APISubscription) {
			subs[0].RegexSelectors[0].Data = utility.ToStringPtr("")
			assert.Error(t, c.SaveSubscriptions("me", []restModel.APISubscription{subs[0]}, false))
		},
		"ValidSubscription": func(t *testing.T, subs []restModel.APISubscription) {
			assert.NoError(t, c.SaveSubscriptions("me", []restModel.APISubscription{subs[0]}, false))
		},
		"ModifyExistingSubscription": func(t *testing.T, subs []restModel.APISubscription) {
			newData := utility.ToStringPtr("5678")
			subs[1].Selectors[0].Data = newData
			assert.NoError(t, c.SaveSubscriptions("my-project", []restModel.APISubscription{subs[1]}, true))

			dbSubs, err := c.GetSubscriptions("my-project", event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, dbSubs, 1)
			require.Equal(t, dbSubs[0].Selectors[0].Data, newData)
		},
		"DisallowedSubscription": func(t *testing.T, subs []restModel.APISubscription) {
			assert.Error(t, c.SaveSubscriptions("me", []restModel.APISubscription{subs[2]}, false))
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))
			subs := []event.Subscription{
				{
					ID:           mgobson.NewObjectId().Hex(),
					Owner:        "my-project",
					OwnerType:    event.OwnerTypeProject,
					ResourceType: "PATCH",
					Trigger:      "outcome",
					Selectors: []event.Selector{
						{
							Type: "id",
							Data: "1234",
						},
					},
					Subscriber: event.Subscriber{
						Type:   event.EmailSubscriberType,
						Target: "a@domain.invalid",
					},
				},
				{
					ID:           mgobson.NewObjectId().Hex(),
					Owner:        "not-my-project",
					OwnerType:    event.OwnerTypeProject,
					ResourceType: "PATCH",
					Trigger:      "outcome",
					Selectors: []event.Selector{
						{
							Type: "id",
							Data: "1234",
						},
					},
					Subscriber: event.Subscriber{
						Type:   event.EmailSubscriberType,
						Target: "a@domain.invalid",
					},
				},
			}
			newSubscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
				Trigger:      utility.ToStringPtr("outcome"),
				Owner:        utility.ToStringPtr("me"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypePerson)),
				RegexSelectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr("object"),
						Data: utility.ToStringPtr("object_data"),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			disallowedSubscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
				Trigger:      utility.ToStringPtr("outcome"),
				Owner:        utility.ToStringPtr("me"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
				RegexSelectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr("object"),
						Data: utility.ToStringPtr("object_data"),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.JIRACommentSubscriberType),
					Target: "ticket",
				},
			}
			for _, sub := range subs {
				assert.NoError(t, sub.Upsert())
			}
			existingSub := restModel.APISubscription{}
			assert.NoError(t, existingSub.BuildFromService(subs[0]))
			test(t, []restModel.APISubscription{newSubscription, existingSub, disallowedSubscription})
		})
	}
}

func TestDeleteProjectSubscriptions(t *testing.T) {
	c := &DBSubscriptionConnector{}
	for name, test := range map[string]func(t *testing.T, ids []string){
		"InvalidOwner": func(t *testing.T, ids []string) {
			assert.Error(t, c.DeleteSubscriptions("my-project", ids))
		},
		"ValidOwner": func(t *testing.T, ids []string) {
			assert.NoError(t, c.DeleteSubscriptions("my-project", []string{ids[0]}))
			subs, err := event.FindSubscriptionsByOwner("my-project", event.OwnerTypeProject)
			assert.NoError(t, err)
			assert.Len(t, subs, 0)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))
			subs := []event.Subscription{
				{
					ID:           mgobson.NewObjectId().Hex(),
					Owner:        "my-project",
					OwnerType:    event.OwnerTypeProject,
					ResourceType: "PATCH",
					Trigger:      "outcome",
					Selectors: []event.Selector{
						{
							Type: "id",
							Data: "1234",
						},
					},
					Subscriber: event.Subscriber{
						Type:   event.EmailSubscriberType,
						Target: "a@domain.invalid",
					},
				},
				{
					ID:           mgobson.NewObjectId().Hex(),
					Owner:        "not-my-project",
					OwnerType:    event.OwnerTypeProject,
					ResourceType: "PATCH",
					Trigger:      "outcome",
					Selectors: []event.Selector{
						{
							Type: "id",
							Data: "1234",
						},
					},
					Subscriber: event.Subscriber{
						Type:   event.EmailSubscriberType,
						Target: "a@domain.invalid",
					},
				},
			}
			toDelete := []string{}
			for _, sub := range subs {
				assert.NoError(t, sub.Upsert())
				toDelete = append(toDelete, sub.ID)
			}
			test(t, toDelete)
		})
	}
}

func TestCopyProjectSubscriptions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))

	subs := []event.Subscription{
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "my-project",
			OwnerType:    event.OwnerTypeProject,
			ResourceType: "PATCH",
			Trigger:      "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "not-my-project",
			OwnerType:    event.OwnerTypeProject,
			ResourceType: "PATCH",
			Trigger:      "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
	}
	for _, sub := range subs {
		assert.NoError(t, sub.Upsert())
	}
	c := &DBSubscriptionConnector{}

	for name, test := range map[string]func(t *testing.T){
		"FromNonExistentProject": func(t *testing.T) {
			assert.NoError(t, c.CopyProjectSubscriptions("not-a-project", "my-new-project"))
			apiSubs, err := event.FindSubscriptionsByOwner("my-new-project", event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, apiSubs, 0)
		},
		"FromExistentProject": func(t *testing.T) {
			assert.NoError(t, c.CopyProjectSubscriptions("my-project", "my-newest-project"))
			apiSubs, err := event.FindSubscriptionsByOwner("my-project", event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, apiSubs, 1)
			assert.Equal(t, subs[0].ID, apiSubs[0].ID)

			apiSubs, err = event.FindSubscriptionsByOwner("my-newest-project", event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, apiSubs, 1)
			assert.NotEqual(t, subs[0].ID, apiSubs[0].ID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			test(t)
		})
	}

}
