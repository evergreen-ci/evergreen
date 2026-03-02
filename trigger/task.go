package trigger

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeTask, event.TaskStarted, makeTaskTriggers)
	registry.registerEventHandler(event.ResourceTypeTask, event.TaskFinished, makeTaskTriggers)
	registry.registerEventHandler(event.ResourceTypeTask, event.TaskBlocked, makeTaskTriggers)
}

const (
	triggerTaskFirstFailureInBuild           = "first-failure-in-build"
	triggerTaskFirstFailureInVersionWithName = "first-failure-in-version-with-name"
	triggerTaskRegressionByTest              = "regression-by-test"
	triggerBuildBreak                        = "build-break"
	keyFailureType                           = "failure-type"
	triggerTaskFailedOrBlocked               = "task-failed-or-blocked"
)

func makeTaskTriggers() eventHandler {
	t := &taskTriggers{
		oldTestResults: map[string]*testresult.TestResult{},
	}
	t.base.triggers = map[string]trigger{
		event.TriggerOutcome:                     t.taskOutcome,
		event.TriggerFailure:                     t.taskFailure,
		event.TriggerSuccess:                     t.taskSuccess,
		event.TriggerExceedsDuration:             t.taskExceedsDuration,
		event.TriggerSuccessfulExceedsDuration:   t.taskSuccessfulExceedsDuration,
		event.TriggerRuntimeChangeByPercent:      t.taskRuntimeChange,
		event.TriggerRegression:                  t.taskRegression,
		event.TriggerTaskFirstFailureInVersion:   t.taskFirstFailureInVersion,
		event.TriggerTaskStarted:                 t.taskStarted,
		triggerTaskFirstFailureInBuild:           t.taskFirstFailureInBuild,
		triggerTaskFirstFailureInVersionWithName: t.taskFirstFailureInVersionWithName,
		triggerTaskRegressionByTest:              t.taskRegressionByTest,
		triggerBuildBreak:                        t.buildBreak,
		triggerTaskFailedOrBlocked:               t.taskFailedOrBlocked,
	}

	return t
}

// newAlertRecord creates an instance of an alert record for the given alert type, populating it
// with as much data from the triggerContext as possible
func newAlertRecord(subID string, t *task.Task, alertType string) *alertrecord.AlertRecord {
	return &alertrecord.AlertRecord{
		Id:                  mgobson.NewObjectId(),
		SubscriptionID:      subID,
		Type:                alertType,
		ProjectId:           t.Project,
		VersionId:           t.Version,
		RevisionOrderNumber: t.RevisionOrderNumber,
		TaskName:            t.DisplayName,
		Variant:             t.BuildVariant,
		TaskId:              t.Id,
		HostId:              t.HostId,
		AlertTime:           time.Now(),
	}
}

func taskFinishedTwoOrMoreDaysAgo(ctx context.Context, taskID string, sub *event.Subscription) (bool, error) {
	renotify, found := sub.TriggerData[event.RenotifyIntervalKey]
	renotifyInterval, err := strconv.Atoi(renotify)
	if renotify == "" || err != nil || !found {
		renotifyInterval = 48
	}
	query := db.Query(task.ById(taskID)).WithFields(task.FinishTimeKey)
	t, err := task.FindOne(ctx, query)
	if err != nil {
		return false, errors.Wrapf(err, "finding task '%s'", taskID)
	}
	if t == nil {
		t, err = task.FindOneOldWithFields(ctx, task.ById(taskID), task.FinishTimeKey)
		if err != nil {
			return false, errors.Wrapf(err, "finding old task '%s'", taskID)
		}
		if t == nil {
			return false, errors.Errorf("task '%s' not found", taskID)
		}
	}

	return time.Since(t.FinishTime) >= time.Duration(renotifyInterval)*time.Hour, nil
}

func getShouldExecuteError(t, previousTask *task.Task) message.Fields {
	m := message.Fields{
		"source":  "notifications",
		"alert":   "transition to failure",
		"outcome": "no alert",
		"task_id": t.Id,
		"query": map[string]string{
			"display": t.DisplayName,
			"variant": t.BuildVariant,
			"project": t.Project,
		},
		"previous": nil,
	}

	if previousTask != nil {
		m["previous"] = map[string]any{
			"id":          previousTask.Id,
			"variant":     previousTask.BuildVariant,
			"project":     previousTask.Project,
			"finish_time": previousTask.FinishTime,
			"status":      previousTask.Status,
		}
	}

	return m
}

type taskTriggers struct {
	event        *event.EventLogEntry
	data         *event.TaskEventData
	task         *task.Task
	owner        string
	uiConfig     evergreen.UIConfig
	jiraMappings *evergreen.JIRANotificationsConfig
	host         *host.Host
	apiTask      *restModel.APITask

	oldTestResults map[string]*testresult.TestResult

	base
}

func (t *taskTriggers) Process(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.Aborted {
		return nil, nil
	}
	return t.base.Process(ctx, sub)
}

func (t *taskTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var ok bool
	t.data, ok = e.Data.(*event.TaskEventData)
	if !ok {
		return errors.New("expected task event data")
	}

	var err error
	if err = t.uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	t.task, err = task.FindOneIdOldOrNew(ctx, e.ResourceId, t.data.Execution)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s' execution %d", e.ResourceId, t.data.Execution)
	}
	if t.task == nil {
		return errors.Errorf("task '%s' execution %d not found", e.ResourceId, t.data.Execution)
	}

	_, err = t.task.GetDisplayTask(ctx)
	if err != nil {
		return errors.Wrapf(err, "getting parent display task for task '%s'", t.task.Id)
	}

	author, err := model.GetVersionAuthorID(ctx, t.task.Version)
	if err != nil {
		return errors.Wrapf(err, "getting owner for version '%s'", t.task.Version)
	}
	t.owner = author

	if t.task.HostId != "" {
		t.host, err = host.FindOneId(ctx, t.task.HostId)
		if err != nil {
			return errors.Wrapf(err, "finding host '%s'", t.task.HostId)
		}
	}

	t.apiTask = &restModel.APITask{}
	if err := t.apiTask.BuildFromService(ctx, t.task, &restModel.APITaskArgs{IncludeProjectIdentifier: true, IncludeAMI: true}); err != nil {
		return errors.Wrap(err, "building API task model")
	}

	t.event = e

	t.jiraMappings = &evergreen.JIRANotificationsConfig{}
	return t.jiraMappings.Get(ctx)
}

func (t *taskTriggers) Attributes() event.Attributes {
	attributes := event.Attributes{
		ID:           []string{t.task.Id},
		Object:       []string{event.ObjectTask},
		Project:      []string{t.task.Project},
		InVersion:    []string{t.task.Version},
		InBuild:      []string{t.task.BuildId},
		DisplayName:  []string{t.task.DisplayName},
		BuildVariant: []string{t.task.BuildVariant},
		Requester:    []string{t.task.Requester},
	}

	if t.task.Requester == evergreen.TriggerRequester {
		attributes.Requester = append(attributes.Requester, evergreen.RepotrackerVersionRequester)
	}
	if t.task.Requester == evergreen.GithubPRRequester {
		attributes.Requester = append(attributes.Requester, evergreen.PatchVersionRequester)
	}
	if t.owner != "" {
		attributes.Owner = append(attributes.Owner, t.owner)
	}

	return attributes
}

func (t *taskTriggers) makeData(ctx context.Context, sub *event.Subscription, pastTenseOverride, testNames string) (*commonTemplateData, error) {
	buildDoc, err := build.FindOne(ctx, build.ById(t.task.BuildId))
	if err != nil {
		return nil, errors.Wrapf(err, "finding build '%s' while building email payload", t.task.BuildId)
	}
	if buildDoc == nil {
		return nil, errors.Errorf("build '%s' not found while building email payload", t.task.BuildId)
	}

	projectRef, err := model.FindMergedProjectRef(ctx, t.task.Project, t.task.Version, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref '%s' for version '%s' while building email payload", t.task.Project, t.task.Version)
	}
	if projectRef == nil {
		return nil, errors.Errorf("project ref '%s' for version '%s' not found while building email payload", t.task.Project, t.task.Version)
	}

	displayName := t.task.DisplayName
	status := t.task.Status
	if testNames != "" {
		displayName += fmt.Sprintf(" (%s)", testNames)
	}

	data := commonTemplateData{
		ID:              t.task.Id,
		EventID:         t.event.ID,
		SubscriptionID:  sub.ID,
		DisplayName:     displayName,
		Object:          "task",
		Project:         projectRef.Identifier,
		URL:             taskLink(t.uiConfig.Url, t.task.Id, t.task.Execution),
		PastTenseStatus: status,
		apiModel:        t.apiTask,
		Task:            t.task,
		ProjectRef:      projectRef,
		Build:           buildDoc,
	}
	slackColor := evergreenFailColor

	if len(t.task.OldTaskId) != 0 {
		data.URL = taskLink(t.uiConfig.Url, t.task.OldTaskId, t.task.Execution)
	}

	if data.PastTenseStatus == evergreen.TaskSystemFailed {
		slackColor = evergreenSystemFailColor

	} else if data.PastTenseStatus == evergreen.TaskSucceeded {
		slackColor = evergreenSuccessColor
		data.PastTenseStatus = "succeeded"
	} else if data.PastTenseStatus == evergreen.TaskStarted {
		slackColor = evergreenRunningColor
	}
	if pastTenseOverride != "" {
		data.PastTenseStatus = pastTenseOverride
	}

	attachmentFields := []*message.SlackAttachmentField{
		{
			Title: "Build",
			Value: fmt.Sprintf("<%s|%s>", buildDoc.GetURL(t.uiConfig.Url), t.task.BuildVariant),
		},
		{
			Title: "Version",
			Value: fmt.Sprintf("<%s|%s>", versionLink(
				versionLinkInput{
					uiBase:    t.uiConfig.Url,
					versionID: t.task.Version,
					hasPatch:  evergreen.IsPatchRequester(buildDoc.Requester),
					isChild:   false,
				},
			), t.task.Version),
		},
		{
			Title: "Duration",
			Value: t.task.TimeTaken.String(),
		},
	}
	if t.task.HostId != "" {
		attachmentFields = append(attachmentFields, &message.SlackAttachmentField{
			Title: "Host",
			Value: fmt.Sprintf("<%s|%s>", hostLink(t.uiConfig.Url, t.task.HostId), t.task.HostId),
		})
	}
	data.slack = []message.SlackAttachment{
		{
			Title:     displayName,
			TitleLink: data.URL,
			Color:     slackColor,
			Fields:    attachmentFields,
		},
	}

	return &data, nil
}

func (t *taskTriggers) generate(ctx context.Context, sub *event.Subscription, pastTenseOverride, testNames string) (*notification.Notification, error) {
	var payload any
	if sub.Subscriber.Type == event.JIRAIssueSubscriberType {
		issueSub, ok := sub.Subscriber.Target.(*event.JIRAIssueSubscriber)
		if !ok {
			return nil, errors.Errorf("unexpected target data type %T", sub.Subscriber.Target)
		}
		var err error
		payload, err = t.makeJIRATaskPayload(ctx, sub.ID, issueSub.Project, testNames)
		if err != nil {
			return nil, errors.Wrap(err, "creating Jira payload for task")
		}

	} else {
		data, err := t.makeData(ctx, sub, pastTenseOverride, testNames)
		if err != nil {
			return nil, errors.Wrap(err, "collecting task data")
		}
		data.emailContent = emailTaskContentTemplate

		payload, err = makeCommonPayload(sub, t.Attributes(), data)
		if err != nil {
			return nil, errors.Wrap(err, "building notification")
		}
		if payload == nil {
			return nil, nil
		}
	}
	n, err := notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
	if err != nil {
		return nil, errors.Wrap(err, "creating notification")
	}
	n.SetTaskMetadata(t.task.Id, t.task.Execution)

	return n, nil
}

func (t *taskTriggers) generateWithAlertRecord(ctx context.Context, sub *event.Subscription, alertType, triggerType, pastTenseOverride, testNames string) (*notification.Notification, error) {
	n, err := t.generate(ctx, sub, pastTenseOverride, testNames)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}

	// verify another alert record hasn't been inserted in this time
	rec, err := GetRecordByTriggerType(ctx, sub.ID, triggerType, t.task)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rec != nil {
		return nil, nil
	}

	newRec := newAlertRecord(sub.ID, t.task, alertType)
	grip.Error(message.WrapError(newRec.Insert(ctx), message.Fields{
		"source":  "alert-record",
		"type":    alertType,
		"task_id": t.task.Id,
	}))

	return n, nil
}

func GetRecordByTriggerType(ctx context.Context, subID, triggerType string, t *task.Task) (*alertrecord.AlertRecord, error) {
	var rec *alertrecord.AlertRecord
	var err error
	switch triggerType {
	case triggerTaskFirstFailureInBuild:
		rec, err = alertrecord.FindOne(ctx, alertrecord.ByFirstFailureInVariant(subID, t.Version, t.BuildVariant))
	case event.TriggerTaskFirstFailureInVersion:
		rec, err = alertrecord.FindOne(ctx, alertrecord.ByFirstFailureInVersion(subID, t.Version))
	case triggerTaskFirstFailureInVersionWithName:
		rec, err = alertrecord.FindOne(ctx, alertrecord.ByFirstFailureInTaskType(subID, t.Version, t.DisplayName))
	case triggerBuildBreak:
		rec, err = alertrecord.FindByFirstRegressionInVersion(ctx, subID, t.Version)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "fetching alertrecord for trigger type '%s'", triggerType)
	}
	return rec, nil
}

func (t *taskTriggers) taskOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if t.data.Status != evergreen.TaskSucceeded && !isValidFailedTaskStatus(t.data.Status) {
		return nil, nil
	}

	return t.generate(ctx, sub, "", "")
}

func (t *taskTriggers) taskFailure(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if !matchingFailureType(sub.TriggerData[keyFailureType], t.task.Details.Type) {
		return nil, nil
	}

	if !isValidFailedTaskStatus(t.data.Status) {
		return nil, nil
	}

	// Shouldn't alert on a system unresponsive task that is still retrying.
	if t.task.IsUnfinishedSystemUnresponsive() {
		return nil, nil
	}

	if t.task.Aborted {
		return nil, nil
	}

	return t.generate(ctx, sub, "", "")
}

func (t *taskTriggers) taskSuccess(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if t.data.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	return t.generate(ctx, sub, "", "")
}

func (t *taskTriggers) taskStarted(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if t.data.Status != evergreen.TaskStarted {
		return nil, nil
	}

	return t.generate(ctx, sub, "", "")
}

func (t *taskTriggers) taskFailedOrBlocked(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	// pass in past tense override so that the message reads "has been blocked" rather than building on status
	if t.task.Blocked() {
		return t.generate(ctx, sub, "been blocked", "")

	}

	// check if it's failed instead
	return t.taskFailure(ctx, sub)
}

func (t *taskTriggers) taskFirstFailureInBuild(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.DisplayOnly {
		return nil, nil
	}
	if t.data.Status != evergreen.TaskFailed || t.task.IsUnfinishedSystemUnresponsive() {
		return nil, nil
	}
	rec, err := GetRecordByTriggerType(ctx, sub.ID, triggerTaskFirstFailureInBuild, t.task)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(ctx, sub, alertrecord.FirstVariantFailureId, triggerTaskFirstFailureInBuild, "", "")
}

func (t *taskTriggers) taskFirstFailureInVersion(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.DisplayOnly {
		return nil, nil
	}
	if t.data.Status != evergreen.TaskFailed || t.task.IsUnfinishedSystemUnresponsive() {
		return nil, nil
	}
	rec, err := GetRecordByTriggerType(ctx, sub.ID, event.TriggerTaskFirstFailureInVersion, t.task)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(ctx, sub, alertrecord.FirstVersionFailureId, event.TriggerTaskFirstFailureInVersion, "", "")
}

func (t *taskTriggers) taskFirstFailureInVersionWithName(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskFailed || t.task.IsUnfinishedSystemUnresponsive() {
		return nil, nil
	}
	rec, err := GetRecordByTriggerType(ctx, sub.ID, triggerTaskFirstFailureInVersionWithName, t.task)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(ctx, sub, alertrecord.FirstTaskTypeFailureId, triggerTaskFirstFailureInVersionWithName, "", "")
}

func (t *taskTriggers) taskRegression(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	shouldNotify, alert, err := isTaskRegression(ctx, sub, t.task)
	if err != nil {
		return nil, err
	}
	if !shouldNotify {
		return nil, nil
	}
	n, err := t.generate(ctx, sub, "", "")
	if err != nil {
		return nil, err
	}
	return n, errors.Wrap(alert.Insert(ctx), "processing regression trigger")
}

func isTaskRegression(ctx context.Context, sub *event.Subscription, t *task.Task) (bool, *alertrecord.AlertRecord, error) {
	if t.Status != evergreen.TaskFailed || !utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.Requester) ||
		t.IsUnfinishedSystemUnresponsive() {
		return false, nil, nil
	}

	if !matchingFailureType(sub.TriggerData[keyFailureType], t.Details.Type) {
		return false, nil, nil
	}

	query := db.Query(task.ByBeforeRevisionWithStatusesAndRequesters(t.RevisionOrderNumber,
		evergreen.TaskCompletedStatuses, t.BuildVariant, t.DisplayName, t.Project, evergreen.SystemVersionRequesterTypes)).Sort([]string{"-" + task.RevisionOrderNumberKey})
	previousTask, err := task.FindOne(ctx, query)
	if err != nil {
		return false, nil, errors.Wrap(err, "fetching previous task")
	}

	shouldSend, err := shouldSendTaskRegression(ctx, sub, t, previousTask)
	if err != nil {
		return false, nil, errors.Wrap(err, "determining if we should send notification")
	}
	if !shouldSend {
		return false, nil, nil
	}

	rec := newAlertRecord(sub.ID, t, alertrecord.TaskFailTransitionId)
	rec.RevisionOrderNumber = -1
	if previousTask != nil {
		rec.RevisionOrderNumber = previousTask.RevisionOrderNumber
	}

	return true, rec, nil
}

func shouldSendTaskRegression(ctx context.Context, sub *event.Subscription, t *task.Task, previousTask *task.Task) (bool, error) {
	if t.Status != evergreen.TaskFailed {
		return false, nil
	}
	if previousTask == nil {
		return true, nil
	}

	if previousTask.Status == evergreen.TaskSucceeded {
		// the task transitioned to failure - but we will only trigger an alert if we haven't recorded
		// a sent alert for a transition after the same previously passing task.
		q := alertrecord.ByLastFailureTransition(sub.ID, t.DisplayName, t.BuildVariant, t.Project)
		lastAlerted, err := alertrecord.FindOne(ctx, q)
		if err != nil {
			errMessage := getShouldExecuteError(t, previousTask)
			errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
			errMessage["error"] = err.Error()
			grip.Error(errMessage)
			return false, err
		}

		if lastAlerted == nil || (lastAlerted.RevisionOrderNumber < previousTask.RevisionOrderNumber) {
			// Either this alert has never been triggered before, or it was triggered for a
			// transition from failure after an older success than this one - so we need to
			// execute this trigger again.
			errMessage := getShouldExecuteError(t, previousTask)
			errMessage["outcome"] = "sending alert"
			errMessage[message.FieldsMsgName] = "identified transition to failure!"
			grip.Info(errMessage)

			return true, nil
		}
	}
	if previousTask.Status == evergreen.TaskFailed {
		// check if enough time has passed since our last transition alert
		q := alertrecord.ByLastFailureTransition(sub.ID, t.DisplayName, t.BuildVariant, t.Project)
		lastAlerted, err := alertrecord.FindOne(ctx, q)
		if err != nil {
			errMessage := getShouldExecuteError(t, previousTask)
			errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
			errMessage["error"] = err.Error()
			errMessage["lastAlert"] = lastAlerted
			errMessage["outcome"] = "not sending alert"
			grip.Error(errMessage)
			return false, err
		}
		if lastAlerted == nil {
			return true, nil
		}

		if lastAlerted.TaskId == "" {
			maybeSend := sometimes.Quarter()
			errMessage := getShouldExecuteError(t, previousTask)
			errMessage[message.FieldsMsgName] = "empty last alert task_id"
			errMessage["lastAlert"] = lastAlerted
			if maybeSend {
				errMessage["outcome"] = "sending alert (25%)"
			} else {
				errMessage["outcome"] = "not sending alert (75%)"

			}
			grip.Warning(errMessage)

			return maybeSend, nil
		}

		return taskFinishedTwoOrMoreDaysAgo(ctx, lastAlerted.TaskId, sub)
	}
	return false, nil
}

func (t *taskTriggers) taskSuccessfulExceedsDuration(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	return t.taskExceedsDuration(ctx, sub)
}

func (t *taskTriggers) taskExceedsDuration(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	thresholdString, ok := sub.TriggerData[event.TaskDurationKey]
	if !ok {
		return nil, errors.Errorf("subscription '%s' has no task time threshold", sub.ID)
	}
	threshold, err := strconv.Atoi(thresholdString)
	if err != nil {
		return nil, errors.Errorf("subscription '%s' has an invalid time threshold", sub.ID)
	}

	maxDuration := time.Duration(threshold) * time.Second
	if !t.task.StartTime.Add(maxDuration).Before(t.task.FinishTime) {
		return nil, nil
	}
	return t.generate(ctx, sub, fmt.Sprintf("exceeded %d seconds", threshold), "")
}

func (t *taskTriggers) taskRuntimeChange(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if t.task.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	percentString, ok := sub.TriggerData[event.TaskPercentChangeKey]
	if !ok {
		return nil, errors.Errorf("subscription '%s' has no percentage increase", sub.ID)
	}
	percent, err := strconv.ParseFloat(percentString, 64)
	if err != nil {
		return nil, errors.Errorf("subscription '%s' has an invalid percentage", sub.ID)
	}

	lastGreen, err := t.task.PreviousCompletedTask(ctx, t.task.Project, []string{evergreen.TaskSucceeded})
	if err != nil {
		return nil, errors.Wrap(err, "retrieving last green task")
	}
	if lastGreen == nil {
		return nil, nil
	}
	thisTaskDuration := float64(t.task.FinishTime.Sub(t.task.StartTime))
	prevTaskDuration := float64(lastGreen.FinishTime.Sub(lastGreen.StartTime))
	shouldNotify, percentChange := runtimeExceedsThreshold(percent, prevTaskDuration, thisTaskDuration)
	if !shouldNotify {
		return nil, nil
	}
	return t.generate(ctx, sub, fmt.Sprintf("changed in runtime by %.1f%% (over threshold of %s%%)", percentChange, percentString), "")
}

// isValidFailedTaskStatus only matches task display statuses that should
// trigger failed task notifications. For example, it excludes setup
// failures.
func isValidFailedTaskStatus(status string) bool {
	return evergreen.IsFailedTaskStatus(status) && status != evergreen.TaskSetupFailed
}

func isTestStatusRegression(oldStatus, newStatus string) bool {
	switch oldStatus {
	case evergreen.TestSkippedStatus, evergreen.TestSucceededStatus,
		evergreen.TestSilentlyFailedStatus:
		if newStatus == evergreen.TestFailedStatus {
			return true
		}
	}

	return false
}

func testMatchesRegex(testName string, sub *event.Subscription) (bool, error) {
	regex, ok := sub.TriggerData[event.TestRegexKey]
	if !ok || regex == "" {
		return true, nil
	}
	return regexp.MatchString(regex, testName)
}

func (t *taskTriggers) shouldIncludeTest(ctx context.Context, sub *event.Subscription, previousTask *task.Task, currentTask *task.Task, test *testresult.TestResult) (bool, error) {
	if test.Status != evergreen.TestFailedStatus {
		return false, nil
	}

	alertForTask, err := alertrecord.FindByTaskRegressionByTaskTest(ctx, sub.ID, test.GetDisplayTestName(), currentTask.DisplayName, currentTask.BuildVariant, currentTask.Project, currentTask.Id)
	if err != nil {
		return false, errors.Wrap(err, "finding alerts for task test")
	}
	// we've already alerted for this task
	if alertForTask != nil {
		return false, nil
	}

	// a test in a new task is defined as a regression
	if previousTask == nil {
		return true, nil
	}

	oldTestResult, ok := t.oldTestResults[test.GetDisplayTestName()]
	// a new test in an existing task is defined as a regression
	if !ok {
		return true, nil
	}

	if isTestStatusRegression(oldTestResult.Status, test.Status) {
		// try to find a stepback alert
		alertForStepback, err := alertrecord.FindByTaskRegressionTestAndOrderNumber(ctx, sub.ID, test.GetDisplayTestName(), currentTask.DisplayName, currentTask.BuildVariant, currentTask.Project, previousTask.RevisionOrderNumber)
		if err != nil {
			return false, errors.Wrap(err, "getting alert for stepback")
		}
		// never alerted for this regression before
		if alertForStepback == nil {
			return true, nil
		}
	} else {
		mostRecentAlert, err := alertrecord.FindByLastTaskRegressionByTest(ctx, sub.ID, test.GetDisplayTestName(), currentTask.DisplayName, currentTask.BuildVariant, currentTask.Project)
		if err != nil {
			return false, errors.Wrap(err, "getting most recent alert")
		}
		if mostRecentAlert == nil {
			return true, nil
		}
		isOld, err := taskFinishedTwoOrMoreDaysAgo(ctx, mostRecentAlert.TaskId, sub)
		if err != nil {
			return false, errors.Wrap(err, "getting last alert age")
		}
		// resend the alert for this regression if it's past the threshold
		if isOld {
			return true, nil
		}
	}

	return false, nil
}

func (t *taskTriggers) taskRegressionByTest(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if err := t.task.PopulateTestResults(ctx); err != nil {
		return nil, errors.Wrap(err, "populating test results for task")
	}

	if !utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.task.Requester) || !isValidFailedTaskStatus(t.task.Status) {
		return nil, nil
	}
	if !matchingFailureType(sub.TriggerData[keyFailureType], t.task.Details.Type) {
		return nil, nil
	}
	// if no tests, alert only if it's a regression in task status
	if len(t.task.LocalTestResults) == 0 {
		return t.taskRegression(ctx, sub)
	}

	catcher := grip.NewBasicCatcher()
	query := db.Query(task.ByBeforeRevisionWithStatusesAndRequesters(t.task.RevisionOrderNumber,
		evergreen.TaskCompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project, evergreen.SystemVersionRequesterTypes)).Sort([]string{"-" + task.RevisionOrderNumberKey})
	previousCompleteTask, err := task.FindOne(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding previous task")
	}
	if previousCompleteTask != nil {
		if err = previousCompleteTask.PopulateTestResults(ctx); err != nil {
			return nil, errors.Wrapf(err, "populating test results for previous task '%s'", previousCompleteTask.Id)
		}
		t.oldTestResults = mapTestResultsByTestName(previousCompleteTask.LocalTestResults)
	}

	testsToAlert := []testresult.TestResult{}
	hasFailingTest := false
	for _, test := range t.task.LocalTestResults {
		if test.Status != evergreen.TestFailedStatus {
			continue
		}
		hasFailingTest = true
		var match bool
		match, err = testMatchesRegex(test.GetDisplayTestName(), sub)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "test-trigger",
				"message": "bad regex in db",
				"task":    t.task.Id,
				"project": t.task.Project,
			}))
			continue
		}
		if !match {
			continue
		}
		var shouldInclude bool
		shouldInclude, err = t.shouldIncludeTest(ctx, sub, previousCompleteTask, t.task, &test)
		if err != nil {
			catcher.Add(err)
			continue
		}
		if shouldInclude {
			orderNumber := t.task.RevisionOrderNumber
			if previousCompleteTask != nil {
				orderNumber = previousCompleteTask.RevisionOrderNumber
			}
			if err = alertrecord.InsertNewTaskRegressionByTestRecord(ctx, sub.ID, t.task.Id, test.GetDisplayTestName(), t.task.DisplayName, t.task.BuildVariant, t.task.Project, orderNumber); err != nil {
				catcher.Add(err)
				continue
			}
			testsToAlert = append(testsToAlert, test)
		}
	}
	if !hasFailingTest {
		return t.taskRegression(ctx, sub)
	}
	if len(testsToAlert) == 0 {
		return nil, nil
	}
	testNames := ""
	for i, test := range testsToAlert {
		testNames += test.GetDisplayTestName()
		if i != len(testsToAlert)-1 {
			testNames += ", "
		}
	}

	n, err := t.generate(ctx, sub, "", testNames)
	if err != nil {
		return nil, err
	}

	return n, catcher.Resolve()
}

func matchingFailureType(requested, actual string) bool {
	if requested == "any" || requested == "" {
		return true
	}
	return requested == actual
}

func (j *taskTriggers) makeJIRATaskPayload(ctx context.Context, subID, project, testNames string) (*message.JiraIssue, error) {
	return JIRATaskPayload(ctx, JiraIssueParameters{
		SubID:     subID,
		Project:   project,
		UiURL:     j.uiConfig.Url,
		EventID:   j.event.ID,
		TestNames: testNames,
		Mappings:  j.jiraMappings,
		Task:      j.task,
		Host:      j.host,
	})
}

// JiraIssueParameters specify a task payload.
type JiraIssueParameters struct {
	SubID     string
	Project   string
	UiURL     string
	UiV2URL   string
	EventID   string
	TestNames string
	Mappings  *evergreen.JIRANotificationsConfig
	Task      *task.Task
	Host      *host.Host
}

// JIRATaskPayload creates a Jira issue for a given task.
func JIRATaskPayload(ctx context.Context, params JiraIssueParameters) (*message.JiraIssue, error) {
	buildDoc, err := build.FindOne(ctx, build.ById(params.Task.BuildId))
	if err != nil {
		return nil, errors.Wrapf(err, "finding build '%s' while building Jira task payload", params.Task.BuildId)
	}
	if buildDoc == nil {
		return nil, errors.Errorf("build '%s' not found while building Jira task payload", params.Task.BuildId)
	}

	versionDoc, err := model.VersionFindOneId(ctx, params.Task.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "finding version '%s' while building Jira task payload", params.Task.Version)
	}
	if versionDoc == nil {
		return nil, errors.Errorf("version '%s' not found while building Jira task payload", params.Task.Version)
	}

	projectRef, err := model.FindMergedProjectRef(ctx, params.Task.Project, params.Task.Version, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref '%s' for version '%s' while building Jira task payload", params.Task.Version, params.Task.Version)
	}
	if projectRef == nil {
		return nil, errors.Errorf("project ref '%s' for version '%s' not found", params.Task.Project, params.Task.Version)
	}

	data := jiraTemplateData{
		Context:         ctx,
		UIRoot:          params.UiURL,
		UIv2Url:         params.UiV2URL,
		SubscriptionID:  params.SubID,
		EventID:         params.EventID,
		Task:            params.Task,
		Version:         versionDoc,
		Project:         projectRef,
		Build:           buildDoc,
		Host:            params.Host,
		TaskDisplayName: params.Task.DisplayName,
	}
	if params.Task.IsPartOfDisplay(ctx) {
		dt, _ := params.Task.GetDisplayTask(ctx)
		if dt != nil {
			data.TaskDisplayName = dt.DisplayName
		}
	}

	builder := jiraBuilder{
		project:  strings.ToUpper(params.Project),
		mappings: params.Mappings,
		data:     data,
	}

	return builder.build(ctx)
}

// mapTestResultsByTestName creates map of display test name to TestResult
// struct. If multiple tests of the same name exist, this function will return
// a failing test if one existed, otherwise it may return any test with the
// same name.
func mapTestResultsByTestName(results []testresult.TestResult) map[string]*testresult.TestResult {
	m := map[string]*testresult.TestResult{}

	for i, result := range results {
		if testResult, ok := m[result.GetDisplayTestName()]; ok {
			if !isTestStatusRegression(testResult.Status, result.Status) {
				continue
			}
		}
		m[result.GetDisplayTestName()] = &results[i]
	}

	return m
}

func detailStatusToHumanSpeak(status string) string {
	switch status {
	case evergreen.TaskFailed:
		return ""
	case evergreen.TaskSetupFailed:
		return "because a setup task failed"
	case evergreen.TaskTimedOut:
		return "because it timed out"
	case evergreen.TaskSystemUnresponse:
		return "because the system was unresponsive"
	case evergreen.TaskSystemTimedOut:
		return "because the system timed out"
	case evergreen.TaskAborted:
		return "because the task was aborted"
	default:
		return fmt.Sprintf("because of something else (%s)", status)
	}
}

// this is very similar to taskRegression, but different enough
func (t *taskTriggers) buildBreak(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.task.IsPartOfDisplay(ctx) {
		return nil, nil
	}

	if t.task.Status != evergreen.TaskFailed || !utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.task.Requester) {
		return nil, nil
	}

	// Regressions are not actionable if they're caused by a host that was terminated or an agent that died
	if t.task.Details.Description == evergreen.TaskDescriptionStranded || t.task.Details.Description == evergreen.TaskDescriptionHeartbeat {
		return nil, nil
	}

	if t.task.TriggerID != "" && sub.Owner != "" { // don't notify committer for a triggered build
		return nil, nil
	}
	query := db.Query(task.ByBeforeRevisionWithStatusesAndRequesters(t.task.RevisionOrderNumber,
		evergreen.TaskCompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project, evergreen.SystemVersionRequesterTypes)).Sort([]string{"-" + task.RevisionOrderNumberKey})
	previousTask, err := task.FindOne(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding previous task")
	}
	if previousTask != nil && previousTask.Status == evergreen.TaskFailed {
		return nil, nil
	}

	lastAlert, err := GetRecordByTriggerType(ctx, sub.ID, triggerBuildBreak, t.task)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if lastAlert != nil {
		return nil, nil
	}

	n, err := t.generateWithAlertRecord(ctx, sub, alertrecord.FirstRegressionInVersion, triggerBuildBreak, "potentially caused a regression", "")
	if err != nil {
		return nil, err
	}
	return n, nil
}
