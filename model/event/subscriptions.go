package event

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	SubscriptionsCollection = "subscriptions"
)

//nolint: deadcode, megacheck, unused
var (
	subscriptionIDKey             = bsonutil.MustHaveTag(Subscription{}, "ID")
	subscriptionResourceTypeKey   = bsonutil.MustHaveTag(Subscription{}, "ResourceType")
	subscriptionTriggerKey        = bsonutil.MustHaveTag(Subscription{}, "Trigger")
	subscriptionSelectorsKey      = bsonutil.MustHaveTag(Subscription{}, "Selectors")
	subscriptionRegexSelectorsKey = bsonutil.MustHaveTag(Subscription{}, "RegexSelectors")
	subscriptionSubscriberKey     = bsonutil.MustHaveTag(Subscription{}, "Subscriber")
	subscriptionOwnerKey          = bsonutil.MustHaveTag(Subscription{}, "Owner")
	subscriptionOwnerTypeKey      = bsonutil.MustHaveTag(Subscription{}, "OwnerType")
	subscriptionTriggerDataKey    = bsonutil.MustHaveTag(Subscription{}, "TriggerData")
	subscriptionLastUpdatedKey    = bsonutil.MustHaveTag(Subscription{}, "LastUpdated")
)

type OwnerType string

const (
	OwnerTypePerson                         OwnerType = "person"
	OwnerTypeProject                        OwnerType = "project"
	TaskDurationKey                                   = "task-duration-secs"
	TaskPercentChangeKey                              = "task-percent-change"
	BuildDurationKey                                  = "build-duration-secs"
	BuildPercentChangeKey                             = "build-percent-change"
	VersionDurationKey                                = "version-duration-secs"
	VersionPercentChangeKey                           = "version-percent-change"
	TestRegexKey                                      = "test-regex"
	RenotifyIntervalKey                               = "renotify-interval"
	ImplicitSubscriptionPatchOutcome                  = "patch-outcome"
	ImplicitSubscriptionPatchFirstFailure             = "patch-first-failure"
	ImplicitSubscriptionBuildBreak                    = "build-break"
	ImplicitSubscriptionSpawnhostExpiration           = "spawnhost-expiration"
	ImplicitSubscriptionSpawnHostOutcome              = "spawnhost-outcome"

	ImplicitSubscriptionCommitQueue = "commit-queue"
	ObjectTask                      = "task"
	ObjectVersion                   = "version"
	ObjectBuild                     = "build"
	ObjectHost                      = "host"
	ObjectPatch                     = "patch"

	TriggerOutcome                   = "outcome"
	TriggerGithubCheckOutcome        = "github-check-outcome"
	TriggerFailure                   = "failure"
	TriggerSuccess                   = "success"
	TriggerRegression                = "regression"
	TriggerExceedsDuration           = "exceeds-duration"
	TriggerRuntimeChangeByPercent    = "runtime-change"
	TriggerExpiration                = "expiration"
	TriggerPatchStarted              = "started"
	TriggerTaskFirstFailureInVersion = "first-failure-in-version"
	TriggerTaskStarted               = "task-started"
)

type Subscription struct {
	ID             string            `bson:"_id"`
	ResourceType   string            `bson:"type"`
	Trigger        string            `bson:"trigger"`
	Selectors      []Selector        `bson:"selectors,omitempty"`
	RegexSelectors []Selector        `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber        `bson:"subscriber"`
	OwnerType      OwnerType         `bson:"owner_type"`
	Owner          string            `bson:"owner"`
	TriggerData    map[string]string `bson:"trigger_data,omitempty"`
	LastUpdated    time.Time         `bson:"last_updated,omitempty"`
}

type unmarshalSubscription struct {
	ID             string            `bson:"_id"`
	ResourceType   string            `bson:"type"`
	Trigger        string            `bson:"trigger"`
	Selectors      []Selector        `bson:"selectors,omitempty"`
	RegexSelectors []Selector        `bson:"regex_selectors,omitempty"`
	Subscriber     Subscriber        `bson:"subscriber"`
	OwnerType      OwnerType         `bson:"owner_type"`
	Owner          string            `bson:"owner"`
	TriggerData    map[string]string `bson:"trigger_data,omitempty"`
}

func (d *Subscription) UnmarshalBSON(in []byte) error {
	return mgobson.Unmarshal(in, d)
}

func (s *Subscription) SetBSON(raw mgobson.Raw) error {
	temp := unmarshalSubscription{}

	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "error unmarshalling subscriber")
	}

	s.ID = temp.ID
	s.ResourceType = temp.ResourceType
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

const (
	SelectorObject       = "object"
	SelectorID           = "id"
	SelectorProject      = "project"
	SelectorOwner        = "owner"
	SelectorRequester    = "requester"
	SelectorStatus       = "status"
	SelectorDisplayName  = "display-name"
	SelectorBuildVariant = "build-variant"
	SelectorInVersion    = "in-version"
	SelectorInBuild      = "in-build"
)

// FindSubscriptions finds all subscriptions of matching resourceType, and whose
// selectors match the selectors slice
func FindSubscriptions(resourceType string, selectors []Selector) ([]Subscription, error) {
	if len(selectors) == 0 {
		return nil, nil
	}

	pipeline := []bson.M{
		{
			"$match": bson.M{
				subscriptionResourceTypeKey: resourceType,
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
		if err != nil || !matched {
			return false
		}
	}

	return true
}

func findSelector(selectors []Selector, selectorResourceType string) *Selector {
	for i := range selectors {
		if selectors[i].Type == selectorResourceType {
			return &selectors[i]
		}
	}

	return nil
}

func (s *Subscription) Upsert() error {
	if s.ID == "" {
		s.ID = mgobson.NewObjectId().Hex()
	}
	update := bson.M{
		subscriptionResourceTypeKey:   s.ResourceType,
		subscriptionTriggerKey:        s.Trigger,
		subscriptionSelectorsKey:      s.Selectors,
		subscriptionRegexSelectorsKey: s.RegexSelectors,
		subscriptionSubscriberKey:     s.Subscriber,
		subscriptionOwnerKey:          s.Owner,
		subscriptionOwnerTypeKey:      s.OwnerType,
		subscriptionTriggerDataKey:    s.TriggerData,
	}
	if !utility.IsZeroTime(s.LastUpdated) {
		update[subscriptionLastUpdatedKey] = s.LastUpdated
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
		s.ID = c.UpsertedId.(string)
		return nil
	}

	if c.Updated != 1 {
		return errors.New("upsert did not modify any documents")
	}
	return nil
}

func FindSubscriptionByID(id string) (*Subscription, error) {
	out := Subscription{}
	query := bson.M{
		subscriptionIDKey: id,
	}
	if mgobson.IsObjectIdHex(id) {
		query = bson.M{
			"$or": []bson.M{
				query,
				bson.M{
					subscriptionIDKey: mgobson.ObjectIdHex(id),
				},
			},
		}
	}
	err := db.FindOneQ(SubscriptionsCollection, db.Query(query), &out)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subcription by ID")
	}

	return &out, nil
}

func RemoveSubscription(id string) error {
	if id == "" {
		return errors.New("id is not valid, cannot remove")
	}

	return db.Remove(SubscriptionsCollection, bson.M{
		subscriptionIDKey: id,
	})
}

func IsSubscriptionAllowed(sub Subscription) (bool, string) {
	for _, selector := range sub.Selectors {
		if selector.Type == SelectorObject {
			if selector.Data == ObjectBuild || selector.Data == ObjectVersion || selector.Data == ObjectTask {
				if sub.Subscriber.Type == JIRAIssueSubscriberType || sub.Subscriber.Type == EvergreenWebhookSubscriberType {
					return false, fmt.Sprintf("Cannot notify by %s for %s", sub.Subscriber.Type, selector.Data)
				}
			}
		}
	}

	return true, ""
}

func ValidateSelectors(selectors []Selector) (bool, string) {
	for i := range selectors {
		if len(selectors[i].Type) == 0 || len(selectors[i].Data) == 0 {
			return false, "Selector had empty type or data"
		}
	}

	return true, ""
}

func (s *Subscription) Validate() error {
	catcher := grip.NewBasicCatcher()
	if len(s.Selectors)+len(s.RegexSelectors) == 0 {
		catcher.Add(errors.New("must specify at least 1 selector"))
	}
	if s.ResourceType == "" {
		catcher.New("subscription type is required")
	}
	if s.Trigger == "" {
		catcher.New("subscription trigger is required")
	}
	if !IsValidOwnerType(string(s.OwnerType)) {
		catcher.Errorf("%s is not a valid owner type", s.OwnerType)
	}
	// Disallow creating a JIRA comment type subscriber for every task in a project
	if s.OwnerType == OwnerTypeProject &&
		s.ResourceType == ResourceTypeTask &&
		s.Subscriber.Type == JIRACommentSubscriberType {
		catcher.New("JIRA comment subscription not allowed for all tasks in the project")
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
	if versionDurationVal, ok := s.TriggerData[VersionDurationKey]; ok {
		catcher.Add(validatePositiveInt(versionDurationVal))
	}
	if versionPercentVal, ok := s.TriggerData[VersionPercentChangeKey]; ok {
		catcher.Add(validatePositiveFloat(versionPercentVal))
	}
	if buildDurationVal, ok := s.TriggerData[BuildDurationKey]; ok {
		catcher.Add(validatePositiveInt(buildDurationVal))
	}
	if buildPercentVal, ok := s.TriggerData[BuildPercentChangeKey]; ok {
		catcher.Add(validatePositiveFloat(buildPercentVal))
	}
	if testRegex, ok := s.TriggerData[TestRegexKey]; ok {
		catcher.Add(validateRegex(testRegex))
	}
	if renotifyInterval, ok := s.TriggerData[RenotifyIntervalKey]; ok {
		catcher.Add(validatePositiveInt(renotifyInterval))
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

func validateRegex(s string) error {
	regex, err := regexp.Compile(s)
	if regex == nil || err != nil {
		return errors.Wrap(err, "invalid regex")
	}
	return nil
}

func (s *Subscription) String() string {
	id := "???"
	if s.ID != "" {
		id = s.ID
	}

	tmpl := []string{
		fmt.Sprintf("ID: %s", id),
		"",
		fmt.Sprintf("when the '%s' event, matching the '%s' trigger occurs,", s.ResourceType, s.Trigger),
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

func CreateOrUpdateImplicitSubscription(resourceType string, id string,
	subscriber Subscriber, user string) (*Subscription, error) {
	var err error
	var sub *Subscription
	if id != "" {
		sub, err = FindSubscriptionByID(id)
		if err != nil {
			return nil, errors.Wrap(err, "error finding subscription")
		}
	}
	if subscriber.Validate() == nil {
		if sub == nil {
			var temp Subscription
			switch resourceType {
			case ImplicitSubscriptionPatchOutcome:
				temp = NewPatchOutcomeSubscriptionByOwner(user, subscriber)
			case ImplicitSubscriptionPatchFirstFailure:
				temp = NewFirstTaskFailureInVersionSubscriptionByOwner(user, subscriber)
			case ImplicitSubscriptionBuildBreak:
				temp = NewBuildBreakSubscriptionByOwner(user, subscriber)
			case ImplicitSubscriptionSpawnhostExpiration:
				temp = NewSpawnhostExpirationSubscription(user, subscriber)
			case ImplicitSubscriptionSpawnHostOutcome:
				temp = NewSpawnHostOutcomeByOwner(user, subscriber)
			case ImplicitSubscriptionCommitQueue:
				temp = NewCommitQueueSubscriptionByOwner(user, subscriber)
			default:
				return nil, errors.Errorf("unknown subscription resource type: %s", resourceType)
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
		if id != "" {
			if err := RemoveSubscription(id); err != nil {
				return nil, errors.Wrap(err, "error removing subscription")
			}
			sub = nil
		}
	}

	return sub, nil
}

func NewSubscriptionByID(resourceType, trigger, id string, sub Subscriber) Subscription {
	return Subscription{
		ResourceType: resourceType,
		Trigger:      trigger,
		Selectors: []Selector{
			{
				Type: SelectorID,
				Data: id,
			},
		},
		Subscriber: sub,
	}
}

func NewVersionGithubCheckOutcomeSubscription(id string, sub Subscriber) Subscription {
	subscription := NewSubscriptionByID(ResourceTypeVersion, TriggerGithubCheckOutcome, id, sub)
	subscription.LastUpdated = time.Now()
	return subscription
}

func NewExpiringPatchOutcomeSubscription(id string, sub Subscriber) Subscription {
	subscription := NewSubscriptionByID(ResourceTypePatch, TriggerOutcome, id, sub)
	subscription.LastUpdated = time.Now()
	return subscription
}

func NewExpiringPatchSuccessSubscription(id string, sub Subscriber) Subscription {
	subscription := NewSubscriptionByID(ResourceTypePatch, TriggerSuccess, id, sub)
	subscription.LastUpdated = time.Now()
	return subscription
}

func NewParentPatchSubscription(id string, sub Subscriber) Subscription {
	subscription := NewSubscriptionByID(ResourceTypePatch, TriggerOutcome, id, sub)
	subscription.LastUpdated = time.Now()
	return subscription
}

func NewPatchOutcomeSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return NewSubscriptionByOwner(owner, sub, ResourceTypePatch, TriggerOutcome)
}

func NewFirstTaskFailureInVersionSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return Subscription{
		ID:           mgobson.NewObjectId().Hex(),
		ResourceType: ResourceTypeTask,
		Trigger:      TriggerTaskFirstFailureInVersion,
		Selectors: []Selector{
			{
				Type: SelectorOwner,
				Data: owner,
			},
			{
				Type: SelectorRequester,
				Data: evergreen.PatchVersionRequester,
			},
		},
		Subscriber: sub,
	}
}

func NewBuildBreakSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return Subscription{
		ID:           mgobson.NewObjectId().Hex(),
		ResourceType: ResourceTypeTask,
		Trigger:      ImplicitSubscriptionBuildBreak,
		Selectors: []Selector{
			{
				Type: SelectorOwner,
				Data: owner,
			},
			{
				Type: SelectorObject,
				Data: ObjectTask,
			},
			{
				Type: SelectorRequester,
				Data: evergreen.RepotrackerVersionRequester,
			},
		},
		Subscriber: sub,
	}
}

func NewSpawnhostExpirationSubscription(owner string, sub Subscriber) Subscription {
	return NewSubscriptionByOwner(owner, sub, ResourceTypeHost, TriggerExpiration)
}

func NewSubscriptionByOwner(owner string, sub Subscriber, resourceType, trigger string) Subscription {
	return Subscription{
		ID:           mgobson.NewObjectId().Hex(),
		ResourceType: resourceType,
		Trigger:      trigger,
		Selectors: []Selector{
			{
				Type: SelectorOwner,
				Data: owner,
			},
		},
		Subscriber: sub,
	}
}

func NewExpiringBuildOutcomeSubscriptionByVersion(versionID string, sub Subscriber) Subscription {
	return Subscription{
		ResourceType: ResourceTypeBuild,
		Trigger:      TriggerOutcome,
		Selectors: []Selector{
			{
				Type: SelectorInVersion,
				Data: versionID,
			},
		},
		Subscriber:  sub,
		LastUpdated: time.Now(),
	}
}

func NewGithubCheckBuildOutcomeSubscriptionByVersion(versionID string, sub Subscriber) Subscription {
	return Subscription{
		ResourceType: ResourceTypeBuild,
		Trigger:      TriggerGithubCheckOutcome,
		Selectors: []Selector{
			{
				Type: SelectorInVersion,
				Data: versionID,
			},
		},
		Subscriber:  sub,
		LastUpdated: time.Now(),
	}
}

func NewSpawnHostOutcomeByOwner(owner string, sub Subscriber) Subscription {
	return Subscription{
		ResourceType: ResourceTypeHost,
		Trigger:      TriggerOutcome,
		Selectors: []Selector{
			{
				Type: SelectorObject,
				Data: ObjectHost,
			},
			{
				Type: SelectorOwner,
				Data: owner,
			},
		},
		Subscriber: sub,
	}
}

func NewCommitQueueSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return NewSubscriptionByOwner(owner, sub, ResourceTypeCommitQueue, TriggerOutcome)
}
