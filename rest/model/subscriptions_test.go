package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestSubscriptionModels(t *testing.T) {
	assert := assert.New(t)
	subscription := event.Subscription{
		ID:           mgobson.NewObjectId().Hex(),
		ResourceType: "atype",
		Trigger:      "atrigger",
		Owner:        "me",
		OwnerType:    event.OwnerTypePerson,
		Selectors: []event.Selector{
			{
				Type: "type1",
				Data: "data1",
			},
		},
		RegexSelectors: []event.Selector{
			{
				Type: "type2",
				Data: "data2",
			},
			{
				Type: "type3",
				Data: "data3",
			},
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
