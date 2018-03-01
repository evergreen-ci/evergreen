package event

import (
	"regexp"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	SubscriptionsCollection = "subscriptions"
	regexPrefix             = "_regex_"
)

var (
	subscriptionIDKey             = bsonutil.MustHaveTag(Subscription{}, "ID")
	subscriptionTypeKey           = bsonutil.MustHaveTag(Subscription{}, "Type")
	subscriptionTriggerKey        = bsonutil.MustHaveTag(Subscription{}, "Trigger")
	subscriptionSelectorsKey      = bsonutil.MustHaveTag(Subscription{}, "Selectors")
	subscriptionRegexSelectorsKey = bsonutil.MustHaveTag(Subscription{}, "RegexSelectors")
	subscriptionSubscriberKey     = bsonutil.MustHaveTag(Subscription{}, "Subscriber")

	groupedSubscriberTypeKey       = bsonutil.MustHaveTag(GroupedSubscribers{}, "Type")
	groupedSubscriberSubscriberKey = bsonutil.MustHaveTag(GroupedSubscribers{}, "Subscribers")

	subscriberWithRegexKey               = bsonutil.MustHaveTag(SubscriberWithRegex{}, "Subscriber")
	subscriberWithRegexRegexSelectorsKey = bsonutil.MustHaveTag(SubscriberWithRegex{}, "RegexSelectors")
)

type Subscription struct {
	ID             bson.ObjectId `bson:"_id"`
	Type           string        `bson:"type"`
	Trigger        string        `bson:"trigger"`
	Selectors      []Selector    `bson:"selectors,omitempty"`
	RegexSelectors []Selector    `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber    `bson:"subscriber"`
}

type Selector struct {
	Type string `bson:"type"`
	Data string `bson:"data"`
}

type GroupedSubscribers struct {
	Type        string                `bson:"_id"`
	Subscribers []SubscriberWithRegex `bson:"subscribers"`
}

type SubscriberWithRegex struct {
	Subscriber     Subscriber `bson:"subscriber"`
	RegexSelectors []Selector `bson:"regex_selectors"`
}

// FindSubscribers finds all subscriptions that match the given information
func FindSubscribers(subscriptionType, triggerType string, selectors []Selector) ([]GroupedSubscribers, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				subscriptionTypeKey:    subscriptionType,
				subscriptionTriggerKey: triggerType,
			},
		},
		{
			"$project": bson.M{
				subscriptionSubscriberKey:     1,
				subscriptionRegexSelectorsKey: 1,
				"keep": bson.M{
					"$and": []bson.M{
						{
							"$ne": []interface{}{
								"$" + subscriptionSelectorsKey,
								[]interface{}{},
							},
						},
						{
							"$setIsSubset": []interface{}{"$" + subscriptionSelectorsKey, selectors},
						},
					},
				},
			},
		},
		//{
		//	"$addFields": bson.M{
		//		"keep": bson.M{
		//			"$setIsSubset": []interface{}{"$" + subscriptionSelectorsKey, selectors},
		//		},
		//	},
		//},
		{
			"$match": bson.M{
				"keep": true,
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + bsonutil.GetDottedKeyName(subscriptionSubscriberKey, subscriberTypeKey),
				"subscribers": bson.M{
					"$push": bson.M{
						subscriberWithRegexKey:               "$" + subscriptionSubscriberKey,
						subscriberWithRegexRegexSelectorsKey: "$" + subscriptionRegexSelectorsKey,
					},
				},
			},
		},
	}

	out := []GroupedSubscribers{}
	if err := db.Aggregate(SubscriptionsCollection, pipeline, &out); err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	for i := range out {
		subscribers := make([]SubscriberWithRegex, 0, len(out[i].Subscribers))
		for j := range out[i].Subscribers {
			sub := &out[i].Subscribers[j]
			if len(sub.RegexSelectors) > 0 && !regexSelectorsMatch(selectors, sub) {
				continue
			}

			subscribers = append(subscribers, *sub)
		}

		out[i].Subscribers = subscribers
	}

	return out, nil
}

func regexSelectorsMatch(selectors []Selector, s *SubscriberWithRegex) bool {
	for i := range s.RegexSelectors {
		selector := findSelector(selectors, s.RegexSelectors[i].Type)
		if selector == nil {
			return false
		}

		matched, err := regexp.MatchString(s.RegexSelectors[i].Data, selector.Data)
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "notifications-errors",
			"message": "bad regex in db",
		}))
		// TODO swallow regex errors?
		if err != nil || !matched {
			return false
		}
	}

	return true
}

func findSelector(selectors []Selector, selectorType string) *Selector {
	for i := range selectors {
		if selectors[i].Type == selectorType {
			return &selectors[i]
		}
	}

	return nil
}
