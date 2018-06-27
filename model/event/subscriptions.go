package event

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
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
	subscriptionTriggerDataKey    = bsonutil.MustHaveTag(Subscription{}, "TriggerData")
)

type OwnerType string

const (
	OwnerTypePerson                  OwnerType = "person"
	OwnerTypeProject                 OwnerType = "project"
	TaskDurationKey                            = "task-duration-secs"
	TaskPercentChangeKey                       = "task-percent-change"
	BuildDurationKey                           = "build-duration-secs"
	BuildPercentChangeKey                      = "build-percent-change"
	ImplicitSubscriptionPatchOutcome           = "patch-outcome"
	ImplicitSubscriptionBuildBreak             = "build-break"
)

type Subscription struct {
	ID             bson.ObjectId     `bson:"_id"`
	Type           string            `bson:"type"`
	Trigger        string            `bson:"trigger"`
	Selectors      []Selector        `bson:"selectors,omitempty"`
	RegexSelectors []Selector        `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber        `bson:"subscriber"`
	OwnerType      OwnerType         `bson:"owner_type"`
	Owner          string            `bson:"owner"`
	TriggerData    map[string]string `bson:"trigger_data,omitempty"`
}

type unmarshalSubscription struct {
	ID             bson.ObjectId     `bson:"_id"`
	Type           string            `bson:"type"`
	Trigger        string            `bson:"trigger"`
	Selectors      []Selector        `bson:"selectors,omitempty"`
	RegexSelectors []Selector        `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber        `bson:"subscriber"`
	OwnerType      OwnerType         `bson:"owner_type"`
	Owner          string            `bson:"owner"`
	TriggerData    map[string]string `bson:"trigger_data,omitempty"`
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
	s.TriggerData = temp.TriggerData

	return nil
}

type Selector struct {
	Type string `bson:"type"`
	Data string `bson:"data"`
}

// FindSubscriptions finds all subscriptions of matching resourceType, and whose
// selectors match the selectors slice
func FindSubscriptions(resourceType string, selectors []Selector) ([]Subscription, error) {
	if len(selectors) == 0 {
		return nil, nil
	}

	pipeline := []bson.M{
		{
			"$match": bson.M{
				subscriptionTypeKey: resourceType,
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
	}

	rawSubs := []Subscription{}
	if err := db.Aggregate(SubscriptionsCollection, pipeline, &rawSubs); err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	out := []Subscription{}
	for i := range rawSubs {
		if len(rawSubs[i].RegexSelectors) > 0 && !regexSelectorsMatch(selectors, rawSubs[i].RegexSelectors) {
			continue
		}

		out = append(out, rawSubs[i])
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
		subscriptionTriggerDataKey:    s.TriggerData,
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

func FindSubscriptionByIDString(id string) (*Subscription, error) {
	if !bson.IsObjectIdHex(id) {
		return nil, errors.Errorf("%s is not a valid ObjectID", id)
	}
	return FindSubscriptionByID(bson.ObjectIdHex(id))
}

func FindSubscriptionByID(id bson.ObjectId) (*Subscription, error) {
	out := Subscription{}
	err := db.FindOneQ(SubscriptionsCollection, db.Query(bson.M{
		subscriptionIDKey: id,
	}), &out)
	if err == mgo.ErrNotFound {
		return nil, nil

	} else if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subcription by ID")
	}

	return &out, nil
}

func RemoveSubscriptionID(id string) error {
	if !bson.IsObjectIdHex(id) {
		return errors.Errorf("%s is not a valid ObjectID", id)
	}
	return RemoveSubscription(bson.ObjectIdHex(id))
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
	catcher.Add(s.runCustomValidation())
	catcher.Add(s.Subscriber.Validate())
	return catcher.Resolve()
}

func (s *Subscription) runCustomValidation() error {
	catcher := grip.NewBasicCatcher()

	if taskDurationVal, ok := s.TriggerData[TaskDurationKey]; ok {
		catcher.Add(validatePositiveInt(taskDurationVal))
	}
	if taskPercentVal, ok := s.TriggerData[TaskPercentChangeKey]; ok {
		catcher.Add(validatePositiveFloat(taskPercentVal))
	}
	if buildDurationVal, ok := s.TriggerData[BuildDurationKey]; ok {
		catcher.Add(validatePositiveInt(buildDurationVal))
	}
	if buildPercentVal, ok := s.TriggerData[BuildPercentChangeKey]; ok {
		catcher.Add(validatePositiveFloat(buildPercentVal))
	}
	return catcher.Resolve()
}

func validatePositiveInt(s string) error {
	val, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("%s must be a number", s)
	}
	if val < 0 {
		return fmt.Errorf("%d cannot be negative", val)
	}
	return nil
}

func validatePositiveFloat(s string) error {
	val, err := util.TryParseFloat(s)
	if err != nil {
		return err
	}
	if val <= 0 {
		return fmt.Errorf("%f must be positive", val)
	}
	return nil
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

const (
	triggerOutcome = "outcome"
)

func CreateOrUpdateImplicitSubscription(subscriptionType string, id bson.ObjectId,
	subscriber Subscriber, user string) (*Subscription, error) {
	var err error
	var sub *Subscription
	if id.Valid() {
		sub, err = FindSubscriptionByID(id)
		if err != nil {
			return nil, errors.Wrap(err, "error finding subscription")
		}
	}
	if subscriber.Validate() == nil {
		if sub == nil {
			var temp Subscription
			switch subscriptionType {
			case ImplicitSubscriptionPatchOutcome:
				temp = NewPatchOutcomeSubscriptionByOwner(user, subscriber)
			case ImplicitSubscriptionBuildBreak:
				temp = NewBuildBreakSubscriptionByOwner(user, subscriber)
			}
			sub = &temp
		} else {
			sub.Subscriber = subscriber
		}

		sub.OwnerType = OwnerTypePerson
		sub.Owner = user
		if err := sub.Upsert(); err != nil {
			return nil, errors.Wrap(err, "failed to update subscription")
		}
	} else {
		if id.Valid() {
			if err := RemoveSubscription(id); err != nil {
				return nil, errors.Wrap(err, "error removing subscription")
			}
			sub = nil
		}
	}

	return sub, nil
}

func NewPatchOutcomeSubscription(id string, sub Subscriber) Subscription {
	return Subscription{
		Type:    ResourceTypePatch,
		Trigger: triggerOutcome,
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
	return NewSubscriptionByOwner(owner, sub, ResourceTypePatch, triggerOutcome)
}

func NewBuildBreakSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return NewSubscriptionByOwner(owner, sub, ResourceTypeVersion, "regression")
}

func NewSubscriptionByOwner(owner string, sub Subscriber, resourceType, trigger string) Subscription {
	return Subscription{
		ID:      bson.NewObjectId(),
		Type:    resourceType,
		Trigger: trigger,
		Selectors: []Selector{
			{
				Type: "owner",
				Data: owner,
			},
		},
		Subscriber: sub,
	}
}

func NewBuildOutcomeSubscriptionByVersion(versionID string, sub Subscriber) Subscription {
	return Subscription{
		Type:    ResourceTypeBuild,
		Trigger: triggerOutcome,
		Selectors: []Selector{
			{
				Type: "in-version",
				Data: versionID,
			},
		},
		Subscriber: sub,
	}
}
