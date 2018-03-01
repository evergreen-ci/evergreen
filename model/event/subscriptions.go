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

	subAggregationSubscriberKey     = bsonutil.MustHaveTag(subscriptionAggregation{}, "Subscriber")
	subAggregationRegexSelectorsKey = bsonutil.MustHaveTag(subscriptionAggregation{}, "RegexSelectors")
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

type subscriptionAggregation struct {
	Subscriber     Subscriber `bson:"subscriber"`
	RegexSelectors []Selector `bson:"regex_selectors,omitempty"`
}

// FindSubscribers finds all subscriptions that match the given information
func FindSubscribers(subscriptionType, triggerType string, selectors []Selector) ([]Subscriber, error) {
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
	}

	out := []subscriptionAggregation{}
	if err := db.Aggregate(SubscriptionsCollection, pipeline, &out); err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	subs := make([]Subscriber, 0, len(out))
	for i := range out {
		if len(out[i].RegexSelectors) > 0 && !regexSelectorsMatch(selectors, &out[i]) {
			continue
		}
		subs = append(subs, out[i].Subscriber)
	}

	return subs, nil
}

func regexSelectorsMatch(selectors []Selector, s *subscriptionAggregation) bool {
	for i := range s.RegexSelectors {
		selector := findSelector(selectors, s.RegexSelectors[i].Type)
		if selector == nil {
			return false
		}

		matched, err := regexp.MatchString(s.RegexSelectors[i].Data, selector.Data)
		grip.Error(message.WrapError(err, message.Fields{
			"source": "notifications-errors",
		}))
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
