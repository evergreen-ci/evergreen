package trigger

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeVersion, event.VersionStateChange, makeVersionTriggers)
	registry.registerEventHandler(event.ResourceTypeVersion, event.VersionGithubCheckFinished, makeVersionTriggers)
	registry.registerEventHandler(event.ResourceTypeVersion, event.VersionChildrenCompletion, makeVersionTriggers)
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
		event.TriggerFamilyOutcome:          t.versionFamilyOutcome,
		event.TriggerFamilySuccess:          t.versionFamilySuccess,
		event.TriggerFamilyFailure:          t.versionFamilyFailure,
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

func (t *versionTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	t.version, err = model.VersionFindOne(model.VersionById(e.ResourceId))
	if err != nil {
		return errors.Wrapf(err, "finding version '%s'", e.ResourceId)
	}
	if t.version == nil {
		return errors.Errorf("version '%s' not found", e.ResourceId)
	}

	var ok bool
	t.data, ok = e.Data.(*event.VersionEventData)
	if !ok {
		return errors.Errorf("version '%s' contains unexpected data with type %T", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *versionTriggers) Attributes() event.Attributes {
	attributes := event.Attributes{
		ID:        []string{t.version.Id},
		Project:   []string{t.version.Identifier},
		Object:    []string{event.ObjectVersion},
		Requester: []string{t.version.Requester},
	}

	if t.version.Requester == evergreen.TriggerRequester {
		attributes.Requester = append(attributes.Requester, evergreen.RepotrackerVersionRequester)
	}
	if t.version.AuthorID != "" {
		owner := t.version.AuthorID

		// If the author is not explicitly set, it will be parent-patch on patches containing children
		if t.event.EventType == event.VersionChildrenCompletion {
			eventData := t.event.Data.(*event.VersionEventData)
			owner = eventData.Author
		}

		attributes.Owner = append(attributes.Owner, owner)
	}
	return attributes
}

func (t *versionTriggers) makeData(sub *event.Subscription, pastTenseOverride string) (*commonTemplateData, error) {
	api := restModel.APIVersion{}
	api.BuildFromService(*t.version)
	projectName := t.version.Identifier
	if api.ProjectIdentifier != nil {
		projectName = utility.FromStringPtr(api.ProjectIdentifier)
	}

	versionStatus := t.data.Status
	if evergreen.IsPatchRequester(t.version.Requester) {
		var err error
		p, err := patch.FindOneId(t.version.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "getting patch for version '%s'", t.version.Id)
		}
		if p == nil {
			return nil, errors.Errorf("no patch found for version '%s'", t.version.Id)
		}

		// Look at collective status because we don't know whether the last patch to finish in the version was a child or a parent.
		versionStatus, err = p.CollectiveStatus()
		if err != nil {
			return nil, errors.Wrap(err, "getting collective status for patch")
		}
		grip.NoticeWhen(versionStatus != t.data.Status, message.Fields{
			"message":                   "patch's current collective status does not match the version event data's status",
			"version_collective_status": versionStatus,
			"version_event_status":      t.data.Status,
			"patch_and_version_id":      t.version.Id,
			"version_status":            t.version.Status,
			"subscription":              sub.ID,
		})
	}

	data := commonTemplateData{
		ID:             t.version.Id,
		EventID:        t.event.ID,
		SubscriptionID: sub.ID,
		DisplayName:    t.version.Id,
		Object:         event.ObjectVersion,
		Project:        projectName,
		URL: versionLink(versionLinkInput{
			uiBase:    t.uiConfig.Url,
			versionID: t.version.Id,
			hasPatch:  evergreen.IsPatchRequester(t.version.Requester),
			isChild:   false,
		}),
		PastTenseStatus:   versionStatus,
		apiModel:          &api,
		githubState:       message.GithubStatePending,
		githubContext:     thirdparty.GithubStatusDefaultContext,
		githubDescription: evergreen.PRTasksRunningDescription,
	}
	if t.data.GithubCheckStatus != "" {
		data.PastTenseStatus = t.data.GithubCheckStatus
	}
	finishTime := t.version.FinishTime
	if utility.IsZeroTime(finishTime) {
		finishTime = time.Now() // this might be true for GitHub check statuses
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
		return nil, errors.Wrap(err, "collecting version data")
	}
	payload, err := makeCommonPayload(sub, t.Attributes(), data)
	if err != nil {
		return nil, errors.Wrap(err, "building notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}
func (t *versionTriggers) versionOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) || t.event.EventType == event.VersionChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *versionTriggers) versionGithubCheckOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *versionTriggers) versionFailure(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || t.event.EventType == event.VersionChildrenCompletion {
		return nil, nil
	}
	failedTasks, err := task.FindAll(db.Query(task.FailedTasksByVersion(t.version.Id)))
	if err != nil {
		return nil, errors.Wrapf(err, "getting failed tasks in version '%s'", t.version.Id)
	}
	skipNotification := false
	for _, failedTask := range failedTasks {
		if !failedTask.Aborted {
			skipNotification = false
			break
		} else {
			skipNotification = true
		}
	}
	if skipNotification {
		return nil, nil
	}
	return t.generate(sub, "")
}

func (t *versionTriggers) versionSuccess(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded || t.event.EventType == event.VersionChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *versionTriggers) versionExceedsDuration(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) {
		return nil, nil
	}
	thresholdString, ok := sub.TriggerData[event.VersionDurationKey]
	if !ok {
		return nil, errors.Errorf("subscription '%s' has no build time threshold", sub.ID)
	}
	threshold, err := strconv.Atoi(thresholdString)
	if err != nil {
		return nil, errors.Errorf("subscription '%s' has an invalid time threshold", sub.ID)
	}

	maxDuration := time.Duration(threshold) * time.Second
	if !t.version.StartTime.Add(maxDuration).Before(t.version.FinishTime) {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("exceeded %d seconds", threshold))
}
func (t *versionTriggers) versionFamilyOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) {
		return nil, nil
	}
	if t.event.EventType != event.VersionChildrenCompletion {
		return nil, nil
	}
	return t.generate(sub, "")
}

func (t *versionTriggers) versionFamilyFailure(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || t.event.EventType != event.VersionChildrenCompletion {
		return nil, nil
	}
	failedTasks, err := task.FindAll(db.Query(task.FailedTasksByVersion(t.version.Id)))
	if err != nil {
		return nil, errors.Wrapf(err, "getting failed tasks in version '%s'", t.version.Id)
	}
	skipNotification := false
	for _, failedTask := range failedTasks {
		if !failedTask.Aborted {
			skipNotification = false
			break
		} else {
			skipNotification = true
		}
	}
	if skipNotification {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *versionTriggers) versionFamilySuccess(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded || t.event.EventType != event.VersionChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *versionTriggers) versionRuntimeChange(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) {
		return nil, nil
	}
	percentString, ok := sub.TriggerData[event.VersionPercentChangeKey]
	if !ok {
		return nil, fmt.Errorf("subscription '%s' has no percentage increase", sub.ID)
	}
	percent, err := strconv.ParseFloat(percentString, 64)
	if err != nil {
		return nil, fmt.Errorf("subscription '%s' has an invalid percentage", sub.ID)
	}

	lastGreen, err := t.version.LastSuccessful()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving last green build")
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

func (t *versionTriggers) versionRegression(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || !utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.version.Requester) {
		return nil, nil
	}

	versionTasks, err := task.FindAll(db.Query(task.ByVersion(t.version.Id)))
	if err != nil {
		return nil, errors.Wrapf(err, "finding tasks for version '%s'", t.version.Id)
	}
	for i := range versionTasks {
		task := &versionTasks[i]
		isRegression, _, err := isTaskRegression(sub, task)
		if err != nil {
			return nil, errors.Wrap(err, "evaluating task regression")
		}
		if isRegression {
			return t.generate(sub, "")
		}
	}
	return nil, nil
}
