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
)

//nolint: deadcode, megacheck
var (
	subscriptionIDKey             = bsonutil.MustHaveTag(Subscription{}, "ID")
	subscriptionTypeKey           = bsonutil.MustHaveTag(Subscription{}, "Type")
	subscriptionTriggerKey        = bsonutil.MustHaveTag(Subscription{}, "Trigger")
	subscriptionSelectorsKey      = bsonutil.MustHaveTag(Subscription{}, "Selectors")
	subscriptionRegexSelectorsKey = bsonutil.MustHaveTag(Subscription{}, "RegexSelectors")
	subscriptionSubscriberKey     = bsonutil.MustHaveTag(Subscription{}, "Subscriber")
	subscriptionExtraDataKey      = bsonutil.MustHaveTag(Subscription{}, "ExtraData")

	groupedSubscriberTypeKey       = bsonutil.MustHaveTag(groupedSubscribers{}, "Type")
	groupedSubscriberSubscriberKey = bsonutil.MustHaveTag(groupedSubscribers{}, "Subscribers")

	subscriberWithRegexKey               = bsonutil.MustHaveTag(subscriberWithRegex{}, "Subscriber")
	subscriberWithRegexRegexSelectorsKey = bsonutil.MustHaveTag(subscriberWithRegex{}, "RegexSelectors")
)

type Subscription struct {
	ID             bson.ObjectId `bson:"_id"`
	Type           string        `bson:"type"`
	Trigger        string        `bson:"trigger"`
	Selectors      []Selector    `bson:"selectors,omitempty"`
	RegexSelectors []Selector    `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber    `bson:"subscriber"`
	ExtraData      interface{}   `bson:"extra_data,omitempty"`
}

type unmarshalSubscription struct {
	ID             bson.ObjectId `bson:"_id"`
	Type           string        `bson:"type"`
	Trigger        string        `bson:"trigger"`
	Selectors      []Selector    `bson:"selectors,omitempty"`
	RegexSelectors []Selector    `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber    `bson:"subscriber"`
	ExtraData      bson.Raw      `bson:"extra_data,omitempty"`
}

func (s *Subscription) SetBSON(raw bson.Raw) error {
	temp := unmarshalSubscription{}

	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "error unmarshalling subscriber")
	}

	if data := registry.GetExtraData(temp.Type, temp.Trigger); data != nil {
		s.ExtraData = data
	}

	// if ExtraData is a bson null
	if temp.ExtraData.Kind == 0x0A {
		if s.ExtraData != nil {
			s.ExtraData = nil
			return errors.New("error unmarshalling extra data: expected extra data in subscription; found none")
		}

	} else {
		if s.ExtraData == nil {
			return errors.New("error unmarshalling extra data: unexpected extra data in subscription")
		}
		if err := temp.ExtraData.Unmarshal(s.ExtraData); err != nil {
			return errors.Wrap(err, "error unmarshalling extra data")
		}
	}

	s.ID = temp.ID
	s.Type = temp.Type
	s.Trigger = temp.Trigger
	s.Selectors = temp.Selectors
	s.RegexSelectors = temp.RegexSelectors
	s.Subscriber = temp.Subscriber

	return nil
}

type Selector struct {
	Type string `bson:"type"`
	Data string `bson:"data"`
}

type groupedSubscribers struct {
	Type        string                `bson:"_id"`
	Subscribers []subscriberWithRegex `bson:"subscribers"`
}

type subscriberWithRegex struct {
	Subscriber     Subscriber `bson:"subscriber"`
	RegexSelectors []Selector `bson:"regex_selectors"`
}

// FindSubscribers finds all subscriptions that match the given information
func FindSubscribers(subscriptionType, triggerType string, selectors []Selector) (map[string][]Subscriber, error) {
	if len(selectors) == 0 {
		return nil, nil
	}
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
					"$setIsSubset": []interface{}{"$" + subscriptionSelectorsKey, selectors},
				},
			},
		},
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

	gs := []groupedSubscribers{}
	if err := db.Aggregate(SubscriptionsCollection, pipeline, &gs); err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	out := map[string][]Subscriber{}
	for i := range gs {
		subscribers := []Subscriber{}
		for j := range gs[i].Subscribers {
			sub := &gs[i].Subscribers[j]
			if len(sub.RegexSelectors) > 0 && !regexSelectorsMatch(selectors, sub) {
				continue
			}

			subscribers = append(subscribers, sub.Subscriber)
		}
		out[gs[i].Type] = subscribers
	}

	return out, nil
}

func regexSelectorsMatch(selectors []Selector, s *subscriberWithRegex) bool {
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

func (s *Subscription) Upsert() error {
	if len(s.ID.Hex()) == 0 {
		s.ID = bson.NewObjectId()
	}

	c, err := db.Upsert(SubscriptionsCollection, bson.M{
		subscriptionIDKey: s.ID,
	}, bson.M{
		subscriptionTypeKey:           s.Type,
		subscriptionTriggerKey:        s.Trigger,
		subscriptionSelectorsKey:      s.Selectors,
		subscriptionRegexSelectorsKey: s.RegexSelectors,
		subscriptionSubscriberKey:     s.Subscriber,
		subscriptionExtraDataKey:      s.ExtraData,
	})
	if err != nil {
		return err
	}
	if c.UpsertedId != nil {
		s.ID = c.UpsertedId.(bson.ObjectId)
		return nil
	}

	if c.Updated != 1 {
		return errors.New("upsert did not modify any documents")
	}
	return nil
}

func (s *Subscription) Remove() error {
	if len(s.ID.Hex()) == 0 {
		return errors.New("subscription has no ID, cannot remove")
	}

	return db.Remove(SubscriptionsCollection, bson.M{
		subscriptionIDKey: s.ID,
	})
}
