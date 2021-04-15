package trigger

import (
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeVersion, event.VersionStateChange, makeVersionTriggers)
	registry.registerEventHandler(event.ResourceTypeVersion, event.VersionGithubCheckFinished, makeVersionTriggers)
}

type versionTriggers struct {
	event    *event.EventLogEntry
	data     *event.VersionEventData
	version  *model.Version
	uiConfig evergreen.UIConfig

	base
}

func makeVersionTriggers() eventHandler {
	t := &versionTriggers{}
	t.base.triggers = map[string]trigger{
		event.TriggerOutcome:                t.versionOutcome,
		event.TriggerGithubCheckOutcome:     t.versionGithubCheckOutcome,
		event.TriggerFailure:                t.versionFailure,
		event.TriggerSuccess:                t.versionSuccess,
		event.TriggerRegression:             t.versionRegression,
		event.TriggerExceedsDuration:        t.versionExceedsDuration,
		event.TriggerRuntimeChangeByPercent: t.versionRuntimeChange,
	}
	return t
}

func (t *versionTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.version, err = model.VersionFindOne(model.VersionById(e.ResourceId))
	if err != nil {
		return errors.Wrap(err, "failed to fetch version")
	}
	if t.version == nil {
		return errors.New("couldn't find version")
	}

	var ok bool
	t.data, ok = e.Data.(*event.VersionEventData)
	if !ok {
		return errors.Errorf("version '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *versionTriggers) Selectors() []event.Selector {
	selectors := []event.Selector{
		{
			Type: event.SelectorID,
			Data: t.version.Id,
		},
		{
			Type: event.SelectorProject,
			Data: t.version.Identifier,
		},
		{
			Type: event.SelectorObject,
			Data: event.ObjectVersion,
		},
		{
			Type: event.SelectorRequester,
			Data: t.version.Requester,
		},
	}
	if t.version.Requester == evergreen.TriggerRequester {
		selectors = append(selectors, event.Selector{
			Type: event.SelectorRequester,
			Data: evergreen.RepotrackerVersionRequester,
		})
	}
	if t.version.AuthorID != "" {
		selectors = append(selectors, event.Selector{Type: event.SelectorOwner, Data: t.version.AuthorID})
	}
	return selectors
}

func (t *versionTriggers) makeData(sub *event.Subscription, pastTenseOverride string) (*commonTemplateData, error) {
	api := restModel.APIVersion{}
	if err := api.BuildFromService(t.version); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}
	data := commonTemplateData{
		ID:                t.version.Id,
		EventID:           t.event.ID,
		SubscriptionID:    sub.ID,
		DisplayName:       t.version.Id,
		Object:            event.ObjectVersion,
		Project:           t.version.Identifier,
		URL:               versionLink(t.uiConfig.Url, t.version.Id, evergreen.IsPatchRequester(t.version.Requester)),
		PastTenseStatus:   t.data.Status,
		apiModel:          &api,
		githubState:       message.GithubStatePending,
		githubContext:     "evergreen",
		githubDescription: "tasks are running",
	}
	if t.data.GithubCheckStatus != "" {
		data.PastTenseStatus = t.data.GithubCheckStatus
	}
	finishTime := t.version.FinishTime
	if utility.IsZeroTime(finishTime) {
		finishTime = time.Now() // this might be true for github check statuses
	}
	slackColor := evergreenFailColor
	if data.PastTenseStatus == evergreen.VersionSucceeded {
		data.PastTenseStatus = "succeeded"
		slackColor = evergreenSuccessColor
		data.githubState = message.GithubStateSuccess
		data.githubDescription = fmt.Sprintf("version finished in %s", finishTime.Sub(t.version.StartTime).String())
	} else if data.PastTenseStatus == evergreen.VersionFailed {
		data.githubState = message.GithubStateFailure
		data.githubDescription = fmt.Sprintf("version finished in %s", finishTime.Sub(t.version.StartTime).String())
	}

	data.slack = []message.SlackAttachment{
		{
			Title:     "Evergreen Version",
			TitleLink: data.URL,
			Color:     slackColor,
			Text:      t.version.Message,
		},
	}
	if pastTenseOverride != "" {
		data.PastTenseStatus = pastTenseOverride
	}

	return &data, nil
}

func (t *versionTriggers) generate(sub *event.Subscription, pastTenseOverride string) (*notification.Notification, error) {
	data, err := t.makeData(sub, pastTenseOverride)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect version data")
	}
	payload, err := makeCommonPayload(sub, t.Selectors(), data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *versionTriggers) versionOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded && t.data.Status != evergreen.VersionFailed {
		return nil, nil
	}

	isReady, err := t.waitOnChildrenOrSiblings(sub)
	if err != nil {
		return nil, err
	}
	if !isReady {
		return nil, nil
	}
	return t.generate(sub, "")
}

func (t *versionTriggers) versionGithubCheckOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.GithubCheckStatus != evergreen.VersionSucceeded && t.data.GithubCheckStatus != evergreen.VersionFailed {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *versionTriggers) versionFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed {
		return nil, nil
	}

	isReady, err := t.waitOnChildrenOrSiblings(sub)
	if err != nil {
		return nil, err
	}
	if !isReady {
		return nil, nil
	}
	return t.generate(sub, "")
}

func (t *versionTriggers) versionSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded {
		return nil, nil
	}

	isReady, err := t.waitOnChildrenOrSiblings(sub)
	if err != nil {
		return nil, err
	}
	if !isReady {
		return nil, nil
	}
	return t.generate(sub, "")
}

func (t *versionTriggers) versionExceedsDuration(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded && t.data.Status != evergreen.VersionFailed {
		return nil, nil
	}
	thresholdString, ok := sub.TriggerData[event.VersionDurationKey]
	if !ok {
		return nil, fmt.Errorf("subscription %s has no build time threshold", sub.ID)
	}
	threshold, err := strconv.Atoi(thresholdString)
	if err != nil {
		return nil, fmt.Errorf("subscription %s has an invalid time threshold", sub.ID)
	}

	maxDuration := time.Duration(threshold) * time.Second
	if !t.version.StartTime.Add(maxDuration).Before(t.version.FinishTime) {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("exceeded %d seconds", threshold))
}

func (t *versionTriggers) versionRuntimeChange(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded && t.data.Status != evergreen.VersionFailed {
		return nil, nil
	}
	percentString, ok := sub.TriggerData[event.VersionPercentChangeKey]
	if !ok {
		return nil, fmt.Errorf("subscription %s has no percentage increase", sub.ID)
	}
	percent, err := strconv.ParseFloat(percentString, 64)
	if err != nil {
		return nil, fmt.Errorf("subscription %s has an invalid percentage", sub.ID)
	}

	lastGreen, err := t.version.LastSuccessful()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving last green build")
	}
	if lastGreen == nil {
		return nil, nil
	}
	thisVersionDuration := float64(t.version.FinishTime.Sub(t.version.StartTime))
	prevVersionDuration := float64(lastGreen.FinishTime.Sub(lastGreen.StartTime))
	shouldNotify, percentChange := runtimeExceedsThreshold(percent, prevVersionDuration, thisVersionDuration)
	if !shouldNotify {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("changed in runtime by %.1f%% (over threshold of %s%%)", percentChange, percentString))
}

func (t *versionTriggers) versionRegression(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || !utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.version.Requester) {
		return nil, nil
	}

	versionTasks, err := task.FindAll(task.ByVersion(t.version.Id))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving tasks for version")
	}
	for i := range versionTasks {
		task := &versionTasks[i]
		isRegression, _, err := isTaskRegression(sub, task)
		if err != nil {
			return nil, errors.Wrap(err, "error evaluating task regression")
		}
		if isRegression {
			return t.generate(sub, "")
		}
	}
	return nil, nil
}

func (t *versionTriggers) waitOnChildrenOrSiblings(sub *event.Subscription) (bool, error) {
	if !(t.version.IsParent || t.version.IsChild()) {
		return true, nil
	}
	isReady := false
	patchDoc, _ := patch.FindOne(patch.ByVersion(t.version.Id))
	// get the children or siblings to wait on
	childrenOrSiblings, parentPatch, err := patchDoc.GetPatchFamily()
	if err != nil {
		return isReady, errors.Wrap(err, "error getting child or sibling patches")
	}

	// make sure the parent is done, if not, wait for the parent
	if t.version.IsChild() {
		if !evergreen.IsFinishedPatchStatus(parentPatch.Status) {
			return isReady, nil
		}
	}

	childrenStatus, err := getChildrenOrSiblingsReadiness(childrenOrSiblings)
	if err != nil {
		return isReady, errors.Wrap(err, "error getting child or sibling information")
	}
	//make sure the children or siblings are done before sending the notification
	if !evergreen.IsFinishedPatchStatus(childrenStatus) {
		return isReady, nil
	}
	isReady = true
	if childrenStatus == evergreen.PatchFailed {
		t.data.Status = evergreen.PatchFailed
	}

	if t.version.IsChild() {
		parentVersion, err := t.version.GetParentVersion()
		if err != nil {
			return isReady, errors.Wrap(err, "error getting parentVersion")
		}
		if parentVersion == nil {
			return isReady, errors.Wrap(err, "error finding parentVersion")
		}
		t.version = parentVersion

	}

	return isReady, nil
}
