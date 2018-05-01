package event

import (
	"fmt"
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
	subscriptionOwnerKey          = bsonutil.MustHaveTag(Subscription{}, "Owner")
	subscriptionOwnerTypeKey      = bsonutil.MustHaveTag(Subscription{}, "OwnerType")

	groupedSubscriberTypeKey       = bsonutil.MustHaveTag(groupedSubscribers{}, "Type")
	groupedSubscriberSubscriberKey = bsonutil.MustHaveTag(groupedSubscribers{}, "Subscribers")

	subscriberWithRegexKey               = bsonutil.MustHaveTag(subscriberWithRegex{}, "Subscriber")
	subscriberWithRegexRegexSelectorsKey = bsonutil.MustHaveTag(subscriberWithRegex{}, "RegexSelectors")
)

type OwnerType string

const (
	OwnerTypePerson  OwnerType = "person"
	OwnerTypeProject           = "project"
)

type Subscription struct {
	ID             bson.ObjectId `bson:"_id"`
	Type           string        `bson:"type"`
	Trigger        string        `bson:"trigger"`
	Selectors      []Selector    `bson:"selectors,omitempty"`
	RegexSelectors []Selector    `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber    `bson:"subscriber"`
	Owner          string        `bson:"owner"`
	OwnerType      OwnerType     `bson:"owner_type"`
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

	// note: this prevents changing the owner of an existing subscription, which is desired
	c, err := db.Upsert(SubscriptionsCollection, bson.M{
		subscriptionIDKey:    s.ID,
		subscriptionOwnerKey: s.Owner,
	},
		bson.M{
			subscriptionTypeKey:           s.Type,
			subscriptionTriggerKey:        s.Trigger,
			subscriptionSelectorsKey:      s.Selectors,
			subscriptionRegexSelectorsKey: s.RegexSelectors,
			subscriptionSubscriberKey:     s.Subscriber,
			subscriptionOwnerKey:          s.Owner,
			subscriptionOwnerTypeKey:      s.OwnerType,
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

func (s *Subscription) Validate() error {
	catcher := grip.NewBasicCatcher()
	if len(s.Selectors)+len(s.RegexSelectors) == 0 {
		catcher.Add(errors.New("must specify at least 1 selector"))
	}
	if s.Type == "" {
		catcher.Add(errors.New("subscription type is required"))
	}
	if s.Trigger == "" {
		catcher.Add(errors.New("subscription trigger is required"))
	}
	catcher.Add(s.Subscriber.Validate())
	return catcher.Resolve()
}

func (s *Subscription) String() string {
	id := "???"
	if s.ID.Valid() {
		id = s.ID.Hex()
	}

	tmpl := []string{
		fmt.Sprintf("ID: %s", id),
		"",
		fmt.Sprintf("when the '%s' event, matching the '%s' trigger occurs,", s.Type, s.Trigger),
		"and the following attributes match:",
	}

	for i := range s.Selectors {
		tmpl = append(tmpl, fmt.Sprintf("\t%s: %s", s.Selectors[i].Type, s.Selectors[i].Data))
	}
	for i := range s.RegexSelectors {
		tmpl = append(tmpl, fmt.Sprintf("\t%s: %s", s.RegexSelectors[i].Type, s.RegexSelectors[i].Data))
	}
	tmpl = append(tmpl, "", "issue the following notification:",
		fmt.Sprintf("\t%s", s.Subscriber))

	out := ""
	for i := range tmpl {
		out += tmpl[i]
		out += "\n"
	}

	return out
}

func FindSubscriptionsByOwner(owner string, ownerType OwnerType) ([]Subscription, error) {
	if len(owner) == 0 {
		return nil, nil
	}
	if !IsValidOwnerType(string(ownerType)) {
		return nil, errors.Errorf("%s is not a valid owner type", ownerType)
	}
	query := db.Query(bson.M{
		subscriptionOwnerKey:     owner,
		subscriptionOwnerTypeKey: ownerType,
	})
	subscriptions := []Subscription{}
	err := db.FindAllQ(SubscriptionsCollection, query, &subscriptions)
	return subscriptions, errors.Wrapf(err, "error retrieving subscriptions for owner %s", owner)
}

func IsValidOwnerType(in string) bool {
	switch in {
	case string(OwnerTypePerson):
		return true
	case string(OwnerTypeProject):
		return true
	default:
		return false
	}
}
