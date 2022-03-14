package model

import (
	"testing"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
)

func TestSubscriptionModels(t *testing.T) {
	assert := assert.New(t)

	owner := "me"
	id := "abcde12345"
	project := "mci"

	subscription := event.Subscription{
		ID:           mgobson.NewObjectId().Hex(),
		ResourceType: "atype",
		Trigger:      "atrigger",
		Owner:        owner,
		OwnerType:    event.OwnerTypePerson,
		Selectors: []event.Selector{
			{
				Type: event.SelectorID,
				Data: id,
			},
		},
		RegexSelectors: []event.Selector{
			{
				Type: event.SelectorOwner,
				Data: owner,
			},
			{
				Type: event.SelectorProject,
				Data: project,
			},
		},
		Filter: event.Filter{
			ID: id,
		},
		Subscriber: event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "email message",
		},
	}

	apiSubscription := APISubscription{}
	err := apiSubscription.BuildFromService(subscription)
	assert.NoError(err)

	origSubscription, err := apiSubscription.ToService()
	assert.NoError(err)
	assert.EqualValues(subscription, origSubscription)
}
