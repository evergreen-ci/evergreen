package trigger

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestPatchOutcome(t *testing.T) {
	assert := assert.New(t)

	id := bson.NewObjectId()
	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		ResourceId:   id.Hex(),
	}
	p := patch.Patch{
		Id:      id,
		Project: "test",
		Author:  "someone",
	}
	assert.NoError(p.Insert())

	subs := []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypePatch,
			Trigger: "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: p.Id.Hex(),
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypePatch,
			Trigger: "success",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: p.Id.Hex(),
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypePatch,
			Trigger: "failure",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: p.Id.Hex(),
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
		},
	}
	for i := range subs {
		assert.NoError(subs[i].Upsert())
	}

	n, err := NotificationsFromEvent(&e)
	assert.NoError(err)
	assert.Len(n, 0)

	p.Status = evergreen.PatchSucceeded
	assert.NoError(db.Update(patch.Collection, bson.M{"_id": p.Id}, &p))
	n, err = NotificationsFromEvent(&e)
	assert.NoError(err)
	assert.Len(n, 2)

	p.Status = evergreen.PatchFailed
	assert.NoError(db.Update(patch.Collection, bson.M{"_id": p.Id}, &p))
	n, err = NotificationsFromEvent(&e)
	assert.NoError(err)
	assert.Len(n, 2)
}
