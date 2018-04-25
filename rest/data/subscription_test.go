package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestGetSubscriptions(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(event.SubscriptionsCollection))

	subs := []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Owner:   "someone",
			Type:    "PATCH",
			Trigger: "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.JIRAIssueSubscriberType,
				Target: "a@domain.invalid",
			},
		},
		{
			ID:      bson.NewObjectId(),
			Owner:   "someoneelse",
			Type:    "PATCH",
			Trigger: "outcomeelse",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.JIRAIssueSubscriberType,
				Target: "a@domain.invalid",
			},
		},
	}

	for i := range subs {
		assert.NoError(subs[i].Upsert())
	}

	c := &DBSubscriptionConnector{}
	apiSubs, err := c.GetSubscriptions("someone")
	assert.NoError(err)
	assert.Len(apiSubs, 1)

	apiSubs, err = c.GetSubscriptions("someoneelse")
	assert.NoError(err)
	assert.Len(apiSubs, 1)

	apiSubs, err = c.GetSubscriptions("who")
	assert.NoError(err)
	assert.Len(apiSubs, 0)

	apiSubs, err = c.GetSubscriptions("")
	assert.EqualError(err, "no user provided")
	assert.Len(apiSubs, 0)
}
