package event

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	SubscriptionsCollection = "subscriptions"
)

//nolint:megacheck,unused
var (
	subscriptionIDKey             = bsonutil.MustHaveTag(Subscription{}, "ID")
	subscriptionResourceTypeKey   = bsonutil.MustHaveTag(Subscription{}, "ResourceType")
	subscriptionTriggerKey        = bsonutil.MustHaveTag(Subscription{}, "Trigger")
	subscriptionSelectorsKey      = bsonutil.MustHaveTag(Subscription{}, "Selectors")
	subscriptionRegexSelectorsKey = bsonutil.MustHaveTag(Subscription{}, "RegexSelectors")
	subscriptionFilterKey         = bsonutil.MustHaveTag(Subscription{}, "Filter")
	subscriptionSubscriberKey     = bsonutil.MustHaveTag(Subscription{}, "Subscriber")
	subscriptionOwnerKey          = bsonutil.MustHaveTag(Subscription{}, "Owner")
	subscriptionOwnerTypeKey      = bsonutil.MustHaveTag(Subscription{}, "OwnerType")
	subscriptionTriggerDataKey    = bsonutil.MustHaveTag(Subscription{}, "TriggerData")
	subscriptionLastUpdatedKey    = bsonutil.MustHaveTag(Subscription{}, "LastUpdated")

	filterObjectKey       = bsonutil.MustHaveTag(Filter{}, "Object")
	filterIDKey           = bsonutil.MustHaveTag(Filter{}, "ID")
	filterProjectKey      = bsonutil.MustHaveTag(Filter{}, "Project")
	filterOwnerKey        = bsonutil.MustHaveTag(Filter{}, "Owner")
	filterRequesterKey    = bsonutil.MustHaveTag(Filter{}, "Requester")
	filterStatusKey       = bsonutil.MustHaveTag(Filter{}, "Status")
	filterDisplayNameKey  = bsonutil.MustHaveTag(Filter{}, "DisplayName")
	filterBuildVariantKey = bsonutil.MustHaveTag(Filter{}, "BuildVariant")
	filterInVersionKey    = bsonutil.MustHaveTag(Filter{}, "InVersion")
	filterInBuildKey      = bsonutil.MustHaveTag(Filter{}, "InBuild")
)

type OwnerType string

const (
	OwnerTypePerson                        OwnerType = "person"
	OwnerTypeProject                       OwnerType = "project"
	TaskDurationKey                                  = "task-duration-secs"
	TaskPercentChangeKey                             = "task-percent-change"
	BuildDurationKey                                 = "build-duration-secs"
	BuildPercentChangeKey                            = "build-percent-change"
	VersionDurationKey                               = "version-duration-secs"
	VersionPercentChangeKey                          = "version-percent-change"
	TestRegexKey                                     = "test-regex"
	RenotifyIntervalKey                              = "renotify-interval"
	GeneralSubscriptionPatchOutcome                  = "patch-outcome"
	GeneralSubscriptionPatchFirstFailure             = "patch-first-failure"
	GeneralSubscriptionBuildBreak                    = "build-break"
	GeneralSubscriptionSpawnhostExpiration           = "spawnhost-expiration"
	GeneralSubscriptionSpawnHostOutcome              = "spawnhost-outcome"
	GeneralSubscriptionCommitQueue                   = "commit-queue"

	ObjectTask    = "task"
	ObjectVersion = "version"
	ObjectBuild   = "build"
	ObjectHost    = "host"
	ObjectPatch   = "patch"

	TriggerOutcome = "outcome"
	// TriggerFamilyOutcome indicates that a patch or version completed,
	// and all their child patches (if there are any) have also completed.
	TriggerFamilyOutcome             = "family-outcome"
	TriggerGithubCheckOutcome        = "github-check-outcome"
	TriggerFailure                   = "failure"
	TriggerFamilyFailure             = "family-failure"
	TriggerSuccess                   = "success"
	TriggerFamilySuccess             = "family-success"
	TriggerRegression                = "regression"
	TriggerExceedsDuration           = "exceeds-duration"
	TriggerSuccessfulExceedsDuration = "successful-exceeds-duration"
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
	Filter         Filter            `bson:"filter"`
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
	Filter         Filter            `bson:"filter"`
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
		return errors.Wrap(err, "unmarshalling subscriber")
	}

	s.ID = temp.ID
	s.ResourceType = temp.ResourceType
	s.Trigger = temp.Trigger
	s.Selectors = temp.Selectors
	s.RegexSelectors = temp.RegexSelectors
	s.Filter = temp.Filter
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

// Attributes describes the properties of an event to be matched with subscription filters.
type Attributes struct {
	Object       []string
	ID           []string
	Project      []string
	Owner        []string
	Requester    []string
	Status       []string
	DisplayName  []string
	BuildVariant []string
	InVersion    []string
	InBuild      []string
}

func (a *Attributes) filterQuery() bson.M {
	filterForAttribute := func(values []string) interface{} {
		if len(values) > 0 {
			in := bson.A{nil}
			for _, attribute := range values {
				in = append(in, attribute)
			}
			return bson.M{"$in": in}
		}

		return nil
	}

	return bson.M{
		filterObjectKey:       filterForAttribute(a.Object),
		filterIDKey:           filterForAttribute(a.ID),
		filterProjectKey:      filterForAttribute(a.Project),
		filterOwnerKey:        filterForAttribute(a.Owner),
		filterRequesterKey:    filterForAttribute(a.Requester),
		filterStatusKey:       filterForAttribute(a.Status),
		filterDisplayNameKey:  filterForAttribute(a.DisplayName),
		filterBuildVariantKey: filterForAttribute(a.BuildVariant),
		filterInVersionKey:    filterForAttribute(a.InVersion),
		filterInBuildKey:      filterForAttribute(a.InBuild),
	}
}

func (a *Attributes) isUnset() bool {
	if len(a.Object) > 0 {
		return false
	}
	if len(a.ID) > 0 {
		return false
	}
	if len(a.Project) > 0 {
		return false
	}
	if len(a.Owner) > 0 {
		return false
	}
	if len(a.Requester) > 0 {
		return false
	}
	if len(a.Status) > 0 {
		return false
	}
	if len(a.DisplayName) > 0 {
		return false
	}
	if len(a.BuildVariant) > 0 {
		return false
	}
	if len(a.InVersion) > 0 {
		return false
	}
	if len(a.InBuild) > 0 {
		return false
	}

	return true
}

// ToSelectorMap returns a map of selector types to the values they correspond to in Attributes.
func (a *Attributes) ToSelectorMap() map[string][]string {
	selectorMap := make(map[string][]string)
	if len(a.Object) > 0 {
		selectorMap[SelectorObject] = a.Object
	}
	if len(a.ID) > 0 {
		selectorMap[SelectorID] = a.ID
	}
	if len(a.Project) > 0 {
		selectorMap[SelectorProject] = a.Project
	}
	if len(a.Owner) > 0 {
		selectorMap[SelectorOwner] = a.Owner
	}
	if len(a.Requester) > 0 {
		selectorMap[SelectorRequester] = a.Requester
	}
	if len(a.Status) > 0 {
		selectorMap[SelectorStatus] = a.Status
	}
	if len(a.DisplayName) > 0 {
		selectorMap[SelectorDisplayName] = a.DisplayName
	}
	if len(a.BuildVariant) > 0 {
		selectorMap[SelectorBuildVariant] = a.BuildVariant
	}
	if len(a.InVersion) > 0 {
		selectorMap[SelectorInVersion] = a.InVersion
	}
	if len(a.InBuild) > 0 {
		selectorMap[SelectorInBuild] = a.InBuild
	}

	return selectorMap
}

func (a *Attributes) valuesForSelector(selector string) ([]string, error) {
	switch selector {
	case SelectorObject:
		return a.Object, nil
	case SelectorID:
		return a.ID, nil
	case SelectorProject:
		return a.Project, nil
	case SelectorOwner:
		return a.Owner, nil
	case SelectorRequester:
		return a.Requester, nil
	case SelectorStatus:
		return a.Status, nil
	case SelectorDisplayName:
		return a.DisplayName, nil
	case SelectorBuildVariant:
		return a.BuildVariant, nil
	case SelectorInVersion:
		return a.InVersion, nil
	case SelectorInBuild:
		return a.InBuild, nil
	default:
		return nil, errors.Errorf("unknown selector '%s'", selector)
	}
}

// Filter specifies the event properties that are of interest.
type Filter struct {
	Object       string `bson:"object,omitempty"`
	ID           string `bson:"id,omitempty"`
	Project      string `bson:"project,omitempty"`
	Owner        string `bson:"owner,omitempty"`
	Requester    string `bson:"requester,omitempty"`
	Status       string `bson:"status,omitempty"`
	DisplayName  string `bson:"display_name,omitempty"`
	BuildVariant string `bson:"build_variant,omitempty"`
	InVersion    string `bson:"in_version,omitempty"`
	InBuild      string `bson:"in_build,omitempty"`
}

func (f *Filter) setFieldFromSelector(selector Selector) error {
	switch selector.Type {
	case SelectorObject:
		f.Object = selector.Data
	case SelectorID:
		f.ID = selector.Data
	case SelectorProject:
		f.Project = selector.Data
	case SelectorOwner:
		f.Owner = selector.Data
	case SelectorRequester:
		f.Requester = selector.Data
	case SelectorStatus:
		f.Status = selector.Data
	case SelectorDisplayName:
		f.DisplayName = selector.Data
	case SelectorBuildVariant:
		f.BuildVariant = selector.Data
	case SelectorInVersion:
		f.InVersion = selector.Data
	case SelectorInBuild:
		f.InBuild = selector.Data
	default:
		return errors.Errorf("unknown selector type '%s'", selector.Type)
	}

	return nil
}

// FromSelectors sets the filter's properties from the selectors' data.
func (f *Filter) FromSelectors(selectors []Selector) error {
	catcher := grip.NewBasicCatcher()
	typesSeen := make(map[string]bool)
	for _, selector := range selectors {
		if selector.Type == "" {
			catcher.New("selector has an empty type")
		}
		if selector.Data == "" {
			catcher.Errorf("selector '%s' has no data", selector.Type)
		}

		if typesSeen[selector.Type] {
			catcher.Errorf("selector type '%s' specified more than once", selector.Type)
		}
		typesSeen[selector.Type] = true

		if catcher.HasErrors() {
			continue
		}

		catcher.Wrap(f.setFieldFromSelector(selector), "setting field from selector")
	}

	return catcher.Resolve()
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

// FindSubscriptionsByAttributes finds all subscriptions of matching resourceType, and whose
// filter and regex selectors match the attributes of the event.
func FindSubscriptionsByAttributes(resourceType string, eventAttributes Attributes) ([]Subscription, error) {
	if eventAttributes.isUnset() {
		return nil, nil
	}

	query := bson.M{subscriptionResourceTypeKey: resourceType}
	// A subscription filter specifies the event attributes it should match.
	// If the subscription's filter specifies a field then it must match one of the corresponding trigger attribute's values.
	for field, filter := range eventAttributes.filterQuery() {
		query[bsonutil.GetDottedKeyName(subscriptionFilterKey, field)] = filter
	}

	selectorFiltered := []Subscription{}
	if err := db.FindAllQ(SubscriptionsCollection, db.Query(query), &selectorFiltered); err != nil {
		return nil, errors.Wrap(err, "finding subscriptions for selectors")
	}

	return filterRegexSelectors(selectorFiltered, eventAttributes), nil
}

func filterRegexSelectors(subscriptions []Subscription, eventAttributes Attributes) []Subscription {
	var regexFiltered []Subscription
	for _, sub := range subscriptions {
		if regexSelectorsMatchEvent(sub.RegexSelectors, eventAttributes) {
			regexFiltered = append(regexFiltered, sub)
		}
	}

	return regexFiltered
}

func regexSelectorsMatchEvent(regexSelectors []Selector, eventAttributes Attributes) bool {
	for _, regexSelector := range regexSelectors {
		attributeValues, err := eventAttributes.valuesForSelector(regexSelector.Type)
		if err != nil || len(attributeValues) == 0 {
			return false
		}
		if !regexMatchesValue(regexSelector.Data, attributeValues) {
			return false
		}
	}
	return true
}

func regexMatchesValue(regexString string, values []string) bool {
	regex, err := regexp.Compile(regexString)
	if err != nil {
		return false
	}

	for _, val := range values {
		matched := regex.MatchString(val)
		if matched {
			return true
		}
	}

	return false
}

// CopyProjectSubscriptions copies subscriptions from the first project for the second project.
func CopyProjectSubscriptions(oldProject, newProject string) error {
	subs, err := FindSubscriptionsByOwner(oldProject, OwnerTypeProject)
	if err != nil {
		return errors.Wrapf(err, "finding subscription for project '%s'", oldProject)
	}

	catcher := grip.NewBasicCatcher()
	for _, sub := range subs {
		sub.Owner = newProject
		sub.ID = ""
		for i, selector := range sub.Selectors {
			if selector.Type == SelectorProject && selector.Data == oldProject {
				sub.Selectors[i].Data = newProject
				sub.Filter.Project = newProject
			}
		}
		catcher.Add(sub.Upsert())
	}
	return catcher.Resolve()
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
		subscriptionFilterKey:         s.Filter,
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
				{
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
		return nil, errors.Wrap(err, "fetching subcription by ID")
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

func (s *Subscription) ValidateSelectors() error {
	catcher := grip.NewBasicCatcher()
	if s.Filter == (Filter{}) {
		catcher.New("no filter parameters specified")
	}

	for _, selector := range s.RegexSelectors {
		if selector.Type == "" {
			catcher.New("selector has an empty type")
		}
		if selector.Data == "" {
			catcher.Errorf("selector '%s' has no data", selector.Type)
		}
	}

	// There are certain subscribers we don't allow anymore at certain levels; verify that these aren't being used.
	for _, selector := range s.Selectors {
		if selector.Type == SelectorObject {
			if selector.Data == ObjectBuild || selector.Data == ObjectVersion || selector.Data == ObjectTask {
				if s.Subscriber.Type == JIRAIssueSubscriberType || s.Subscriber.Type == EvergreenWebhookSubscriberType {
					catcher.Errorf("cannot notify by subscriber type '%s' for selector '%s'", s.Subscriber.Type, selector.Data)
				}
			}
		}
	}

	return catcher.Resolve()
}

func (s *Subscription) Validate() error {
	catcher := grip.NewBasicCatcher()
	if s.ResourceType == "" {
		catcher.New("subscription type is required")
	}
	if s.Trigger == "" {
		catcher.New("subscription trigger is required")
	}
	if !IsValidOwnerType(string(s.OwnerType)) {
		catcher.Errorf("'%s' is not a valid owner type", s.OwnerType)
	}

	// Disallow creating a JIRA comment type subscriber for every task in a project.
	if s.OwnerType == OwnerTypeProject &&
		s.ResourceType == ResourceTypeTask &&
		(s.Subscriber.Type == JIRACommentSubscriberType || s.Subscriber.Type == JIRAIssueSubscriberType) {
		catcher.New("JIRA comment/issue subscription not allowed for all tasks in the project")
	}

	catcher.Add(s.ValidateSelectors())
	catcher.Add(s.runCustomValidation())
	catcher.Add(s.Subscriber.Validate())
	return catcher.Resolve()
}

func (s *Subscription) runCustomValidation() error {
	catcher := grip.NewBasicCatcher()

	if taskDurationVal, ok := s.TriggerData[TaskDurationKey]; ok {
		catcher.Wrap(validatePositiveInt(taskDurationVal), "invalid task duration")
	}
	if taskPercentVal, ok := s.TriggerData[TaskPercentChangeKey]; ok {
		catcher.Wrap(validatePositiveFloat(taskPercentVal), "invalid task percentage runtime change")
	}
	if versionDurationVal, ok := s.TriggerData[VersionDurationKey]; ok {
		catcher.Wrap(validatePositiveInt(versionDurationVal), "invalid version duration")
	}
	if versionPercentVal, ok := s.TriggerData[VersionPercentChangeKey]; ok {
		catcher.Wrap(validatePositiveFloat(versionPercentVal), "invalid version percentage runtime change")
	}
	if buildDurationVal, ok := s.TriggerData[BuildDurationKey]; ok {
		catcher.Wrap(validatePositiveInt(buildDurationVal), "invalid build duration")
	}
	if buildPercentVal, ok := s.TriggerData[BuildPercentChangeKey]; ok {
		catcher.Wrap(validatePositiveFloat(buildPercentVal), "invalid build percentage runtime change")
	}
	if testRegex, ok := s.TriggerData[TestRegexKey]; ok {
		catcher.Wrap(validateRegex(testRegex), "invalid test regex")
	}
	if renotifyInterval, ok := s.TriggerData[RenotifyIntervalKey]; ok {
		catcher.Wrap(validatePositiveInt(renotifyInterval), "invalid renotify interval")
	}
	return catcher.Resolve()
}

func validatePositiveInt(s string) error {
	val, err := strconv.Atoi(s)
	if err != nil {
		return errors.Wrapf(err, "invalid number '%s'", s)
	}
	if val < 0 {
		return fmt.Errorf("%d cannot be negative", val)
	}
	return nil
}

func validatePositiveFloat(s string) error {
	val, err := util.TryParseFloat(s)
	if err != nil {
		return errors.Wrapf(err, "invalid number '%s'", s)
	}
	if val <= 0 {
		return fmt.Errorf("%f cannot be negative", val)
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
		return nil, errors.Errorf("'%s' is not a valid owner type", ownerType)
	}
	query := db.Query(bson.M{
		subscriptionOwnerKey:     owner,
		subscriptionOwnerTypeKey: ownerType,
	})
	subscriptions := []Subscription{}
	err := db.FindAllQ(SubscriptionsCollection, query, &subscriptions)
	return subscriptions, errors.Wrapf(err, "retrieving subscriptions for owner '%s'", owner)
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

func CreateOrUpdateGeneralSubscription(resourceType string, id string,
	subscriber Subscriber, user string) (*Subscription, error) {
	var err error
	var sub *Subscription
	if id != "" {
		sub, err = FindSubscriptionByID(id)
		if err != nil {
			return nil, errors.Wrap(err, "finding subscription")
		}
	}
	if subscriber.Validate() == nil {
		if sub == nil {
			var temp Subscription
			switch resourceType {
			case GeneralSubscriptionPatchOutcome:
				temp = NewPatchOutcomeSubscriptionByOwner(user, subscriber)
			case GeneralSubscriptionPatchFirstFailure:
				temp = NewFirstTaskFailureInVersionSubscriptionByOwner(user, subscriber)
			case GeneralSubscriptionBuildBreak:
				temp = NewBuildBreakSubscriptionByOwner(user, subscriber)
			case GeneralSubscriptionSpawnhostExpiration:
				temp = NewSpawnhostExpirationSubscription(user, subscriber)
			case GeneralSubscriptionSpawnHostOutcome:
				temp = NewSpawnHostOutcomeByOwner(user, subscriber)
			case GeneralSubscriptionCommitQueue:
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
			return nil, errors.Wrap(err, "upserting subscription")
		}
	} else {
		if id != "" {
			if err := RemoveSubscription(id); err != nil {
				return nil, errors.Wrap(err, "removing subscription")
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
		Filter:     Filter{ID: id},
		Subscriber: sub,
	}
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
		Filter:     Filter{Owner: owner},
		Subscriber: sub,
	}
}

func NewVersionGithubCheckOutcomeSubscription(id string, sub Subscriber) Subscription {
	subscription := NewSubscriptionByID(ResourceTypeVersion, TriggerGithubCheckOutcome, id, sub)
	subscription.LastUpdated = time.Now()
	return subscription
}

func NewExpiringPatchOutcomeSubscription(id string, sub Subscriber) Subscription {
	subscription := NewSubscriptionByID(ResourceTypePatch, TriggerFamilyOutcome, id, sub)
	subscription.LastUpdated = time.Now()
	return subscription
}

func NewExpiringPatchChildOutcomeSubscription(id string, sub Subscriber) Subscription {
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
	return NewSubscriptionByOwner(owner, sub, ResourceTypePatch, TriggerFamilyOutcome)
}

func NewSpawnhostExpirationSubscription(owner string, sub Subscriber) Subscription {
	return NewSubscriptionByOwner(owner, sub, ResourceTypeHost, TriggerExpiration)
}

func NewCommitQueueSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return NewSubscriptionByOwner(owner, sub, ResourceTypeCommitQueue, TriggerOutcome)
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
		Filter: Filter{
			Owner:     owner,
			Requester: evergreen.PatchVersionRequester,
		},
		Subscriber: sub,
	}
}

func NewBuildBreakSubscriptionByOwner(owner string, sub Subscriber) Subscription {
	return Subscription{
		ID:           mgobson.NewObjectId().Hex(),
		ResourceType: ResourceTypeTask,
		Trigger:      GeneralSubscriptionBuildBreak,
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
		Filter: Filter{
			Owner:     owner,
			Object:    ObjectTask,
			Requester: evergreen.RepotrackerVersionRequester,
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
		Filter:      Filter{InVersion: versionID},
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
		Filter:      Filter{InVersion: versionID},
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
		Filter: Filter{
			Object: ObjectHost,
			Owner:  owner,
		},
		Subscriber: sub,
	}
}
