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

	groupedSubscriptionsTypeKey          = bsonutil.MustHaveTag(groupedSubscriptions{}, "Type")
	groupedSubscriptionsSubscriptionsKey = bsonutil.MustHaveTag(groupedSubscriptions{}, "Subscriptions")
)

type OwnerType string

const (
	OwnerTypePerson  OwnerType = "person"
	OwnerTypeProject OwnerType = "project"
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

type unmarshalSubscription struct {
	ID             bson.ObjectId `bson:"_id"`
	Type           string        `bson:"type"`
	Trigger        string        `bson:"trigger"`
	Selectors      []Selector    `bson:"selectors,omitempty"`
	RegexSelectors []Selector    `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber    `bson:"subscriber"`
	Owner          string        `bson:"owner"`
	OwnerType      OwnerType     `bson:"owner_type"`
}

func (s *Subscription) SetBSON(raw bson.Raw) error {
	temp := unmarshalSubscription{}

	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "error unmarshalling subscriber")
	}

	s.ID = temp.ID
	s.Type = temp.Type
	s.Trigger = temp.Trigger
	s.Selectors = temp.Selectors
	s.RegexSelectors = temp.RegexSelectors
	s.Subscriber = temp.Subscriber
	s.Owner = temp.Owner
	s.OwnerType = temp.OwnerType

	return nil
}

type Selector struct {
	Type string `bson:"type"`
	Data string `bson:"data"`
}

type groupedSubscriptions struct {
	Type          string         `bson:"_id"`
	Subscriptions []Subscription `bson:"subscriptions"`
}

// FindSubscriptions finds all subscriptions that match the given information,
// returning them in a map by subscriber type
func FindSubscriptions(subscriptionType, triggerType string, selectors []Selector) (map[string][]Subscription, error) {
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
			"$addFields": bson.M{
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
				"subscriptions": bson.M{
					"$push": "$$ROOT",
				},
			},
		},
	}

	gs := []groupedSubscriptions{}
	if err := db.Aggregate(SubscriptionsCollection, pipeline, &gs); err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	out := map[string][]Subscription{}
	for i := range gs {
		for j := range gs[i].Subscriptions {
			sub := &gs[i].Subscriptions[j]
			if len(sub.RegexSelectors) > 0 && !regexSelectorsMatch(selectors, sub.RegexSelectors) {
				continue
			}

			out[gs[i].Type] = append(out[gs[i].Type], *sub)
		}
	}

	return out, nil
}

func regexSelectorsMatch(selectors []Selector, regexSelectors []Selector) bool {
	for i := range regexSelectors {
		selector := findSelector(selectors, regexSelectors[i].Type)
		if selector == nil {
			return false
		}

		matched, err := regexp.MatchString(regexSelectors[i].Data, selector.Data)
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
	update := bson.M{
		subscriptionTypeKey:           s.Type,
		subscriptionTriggerKey:        s.Trigger,
		subscriptionSelectorsKey:      s.Selectors,
		subscriptionRegexSelectorsKey: s.RegexSelectors,
		subscriptionSubscriberKey:     s.Subscriber,
		subscriptionOwnerKey:          s.Owner,
		subscriptionOwnerTypeKey:      s.OwnerType,
	}

	// note: this prevents changing the owner of an existing subscription, which is desired
	c, err := db.Upsert(SubscriptionsCollection, bson.M{
		subscriptionIDKey:    s.ID,
		subscriptionOwnerKey: s.Owner,
	}, update)
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

func FindSubscriptionByID(id bson.ObjectId) (*Subscription, error) {
	out := Subscription{}
	err := db.FindOneQ(SubscriptionsCollection, db.Query(bson.M{
		subscriptionIDKey: id,
	}), &out)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subcription by ID")
	}

	return &out, nil
}

func RemoveSubscription(id bson.ObjectId) error {
	if !id.Valid() {
		return errors.New("id is not valid, cannot remove")
	}

	return db.Remove(SubscriptionsCollection, bson.M{
		subscriptionIDKey: id,
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
	if !IsValidOwnerType(string(s.OwnerType)) {
		catcher.Add(errors.Errorf("%s is not a valid owner type", s.OwnerType))
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

func NewPatchOutcomeSubscription(id string, sub Subscriber) Subscription {
	return Subscription{
		Type:    ResourceTypePatch,
		Trigger: "outcome",
		Selectors: []Selector{
			{
				Type: "id",
				Data: id,
			},
		},
		Subscriber: sub,
	}
}

func NewPatchOutcomeSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return Subscription{
		ID:      bson.NewObjectId(),
		Type:    ResourceTypePatch,
		Trigger: "outcome",
		Selectors: []Selector{
			{
				Type: "owner",
				Data: owner,
			},
		},
		Subscriber: sub,
	}
}
