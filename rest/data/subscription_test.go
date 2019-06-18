package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
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

func TestCopyProjectSubscriptions(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(event.SubscriptionsCollection))

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
		assert.NoError(sub.Upsert())
	}
	c := &DBSubscriptionConnector{}

	for name, test := range map[string]func(t *testing.T){
		"FromNonExistentProject": func(t *testing.T) {
			assert.NoError(c.CopyProjectSubscriptions("not-a-project", "my-new-project"))
			apiSubs, err := event.FindSubscriptionsByOwner("my-new-project", event.OwnerTypeProject)
			assert.NoError(err)
			require.Len(apiSubs, 0)
		},
		"FromExistentProject": func(t *testing.T) {
			assert.NoError(c.CopyProjectSubscriptions("my-project", "my-newest-project"))
			apiSubs, err := event.FindSubscriptionsByOwner("my-project", event.OwnerTypeProject)
			assert.NoError(err)
			require.Len(apiSubs, 1)
			assert.Equal(subs[0].ID, apiSubs[0].ID)

			apiSubs, err = event.FindSubscriptionsByOwner("my-newest-project", event.OwnerTypeProject)
			assert.NoError(err)
			require.Len(apiSubs, 1)
			assert.NotEqual(subs[0].ID, apiSubs[0].ID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			test(t)
		})
	}

}
