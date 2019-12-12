package trigger

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeTask, event.TaskFinished, makeTaskTriggers)
}

const (
	triggerTaskFirstFailureInBuild           = "first-failure-in-build"
	triggerTaskFirstFailureInVersion         = "first-failure-in-version"
	triggerTaskFirstFailureInVersionWithName = "first-failure-in-version-with-name"
	triggerTaskRegressionByTest              = "regression-by-test"
	triggerBuildBreak                        = "build-break"
	keyFailureType                           = "failure-type"
)

func makeTaskTriggers() eventHandler {
	t := &taskTriggers{
		oldTestResults: map[string]*task.TestResult{},
	}
	t.base.triggers = map[string]trigger{
		event.TriggerOutcome:                     t.taskOutcome,
		event.TriggerFailure:                     t.taskFailure,
		event.TriggerSuccess:                     t.taskSuccess,
		event.TriggerExceedsDuration:             t.taskExceedsDuration,
		event.TriggerRuntimeChangeByPercent:      t.taskRuntimeChange,
		event.TriggerRegression:                  t.taskRegression,
		triggerTaskFirstFailureInBuild:           t.taskFirstFailureInBuild,
		triggerTaskFirstFailureInVersion:         t.taskFirstFailureInVersion,
		triggerTaskFirstFailureInVersionWithName: t.taskFirstFailureInVersionWithName,
		triggerTaskRegressionByTest:              t.taskRegressionByTest,
		triggerBuildBreak:                        t.buildBreak,
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

func taskFinishedTwoOrMoreDaysAgo(taskID string, sub *event.Subscription) (bool, error) {
	renotify, found := sub.TriggerData[event.RenotifyIntervalKey]
	renotifyInterval, err := strconv.Atoi(renotify)
	if renotify == "" || err != nil || !found {
		renotifyInterval = 48
	}
	t, err := task.FindOneNoMerge(task.ById(taskID).WithFields(task.FinishTimeKey))
	if err != nil {
		return false, errors.Wrapf(err, "error finding task '%s'", taskID)
	}
	if t == nil {
		t, err = task.FindOneOldNoMerge(task.ById(taskID).WithFields(task.FinishTimeKey))
		if err != nil {
			return false, errors.Wrapf(err, "error finding old task '%s'", taskID)
		}
		if t == nil {
			return false, errors.Errorf("task %s not found", taskID)
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
		m["previous"] = map[string]interface{}{
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
	event    *event.EventLogEntry
	data     *event.TaskEventData
	task     *task.Task
	version  *model.Version
	uiConfig evergreen.UIConfig

	oldTestResults map[string]*task.TestResult

	base
}

func (t *taskTriggers) Process(sub *event.Subscription) (*notification.Notification, error) {
	if t.task.DisplayOnly {
		return nil, nil
	}
	if t.task.Aborted {
		return nil, nil
	}

	return t.base.Process(sub)
}

func (t *taskTriggers) Fetch(e *event.EventLogEntry) error {
	var ok bool
	t.data, ok = e.Data.(*event.TaskEventData)
	if !ok {
		return errors.New("expected task event data")
	}

	var err error
	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.task, err = task.FindOneIdOldOrNew(e.ResourceId, t.data.Execution)
	if err != nil {
		return errors.Wrap(err, "failed to fetch task")
	}
	if t.task == nil {
		return errors.New("couldn't find task")
	}

	_, err = t.task.GetDisplayTask()
	if err != nil {
		return errors.Wrap(err, "error getting display task")
	}

	t.version, err = model.VersionFindOne(model.VersionById(t.task.Version))
	if err != nil {
		return errors.Wrap(err, "failed to fetch version")
	}
	if t.version == nil {
		return errors.New("couldn't find version")
	}

	t.event = e

	return nil
}

func (t *taskTriggers) Selectors() []event.Selector {
	selectors := []event.Selector{
		{
			Type: event.SelectorID,
			Data: t.task.Id,
		},
		{
			Type: event.SelectorObject,
			Data: event.ObjectTask,
		},
		{
			Type: event.SelectorProject,
			Data: t.task.Project,
		},
		{
			Type: event.SelectorInVersion,
			Data: t.task.Version,
		},
		{
			Type: event.SelectorInBuild,
			Data: t.task.BuildId,
		},
		{
			Type: event.SelectorDisplayName,
			Data: t.task.DisplayName,
		},
		{
			Type: event.SelectorRequester,
			Data: t.task.Requester,
		},
	}
	if t.task.Requester == evergreen.TriggerRequester {
		selectors = append(selectors, event.Selector{
			Type: event.SelectorRequester,
			Data: evergreen.RepotrackerVersionRequester,
		})
	}
	if t.version != nil && t.version.AuthorID != "" {
		selectors = append(selectors, event.Selector{
			Type: event.SelectorOwner,
			Data: t.version.AuthorID,
		})
	}

	return selectors
}

func (t *taskTriggers) makeData(sub *event.Subscription, pastTenseOverride, testNames string) (*commonTemplateData, error) {
	api := restModel.APITask{}
	if err := api.BuildFromService(t.task); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	buildDoc, err := build.FindOne(build.ById(t.task.BuildId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch build while building email payload")
	}
	if buildDoc == nil {
		return nil, errors.New("could not find build while building email payload")
	}

	projectRef, err := model.FindOneProjectRef(t.task.Project)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch project ref while building email payload")
	}
	if projectRef == nil {
		return nil, errors.New("could not find project ref while building email payload")
	}

	displayName := t.task.DisplayName
	status := t.task.Status
	if t.task.DisplayTask != nil {
		displayName = t.task.DisplayTask.DisplayName
		status = t.task.DisplayTask.Status
	}
	if testNames != "" {
		displayName += fmt.Sprintf(" (%s)", testNames)
	}

	data := commonTemplateData{
		ID:              t.task.Id,
		EventID:         t.event.ID,
		SubscriptionID:  sub.ID,
		DisplayName:     displayName,
		Object:          "task",
		Project:         t.task.Project,
		URL:             taskLink(t.uiConfig.Url, t.task.Id, t.task.Execution),
		PastTenseStatus: status,
		apiModel:        &api,
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
	}
	if pastTenseOverride != "" {
		data.PastTenseStatus = pastTenseOverride
	}

	data.slack = []message.SlackAttachment{
		{
			Title:     displayName,
			TitleLink: data.URL,
			Text:      taskFormat(t.task),
			Color:     slackColor,
		},
	}

	return &data, nil
}

func (t *taskTriggers) generate(sub *event.Subscription, pastTenseOverride, testNames string) (*notification.Notification, error) {
	var payload interface{}
	if sub.Subscriber.Type == event.JIRAIssueSubscriberType {
		issueSub, ok := sub.Subscriber.Target.(*event.JIRAIssueSubscriber)
		if !ok {
			return nil, errors.Errorf("unexpected target data type: '%T'", sub.Subscriber.Target)
		}
		var err error
		payload, err = t.makeJIRATaskPayload(sub.ID, issueSub.Project, testNames)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create jira payload for task")
		}

	} else {
		data, err := t.makeData(sub, pastTenseOverride, testNames)
		if err != nil {
			return nil, errors.Wrap(err, "failed to collect task data")
		}
		data.emailContent = emailTaskContentTemplate

		payload, err = makeCommonPayload(sub, t.Selectors(), data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build notification")
		}
	}
	n, err := notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a notification")
	}
	n.SetTaskMetadata(t.task.Id, t.task.Execution)

	return n, nil
}

func (t *taskTriggers) generateWithAlertRecord(sub *event.Subscription, alertType, pastTenseOverride, testNames string) (*notification.Notification, error) {
	n, err := t.generate(sub, pastTenseOverride, testNames)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}

	rec := newAlertRecord(sub.ID, t.task, alertType)
	grip.Error(message.WrapError(rec.Insert(), message.Fields{
		"source":  "alert-record",
		"type":    alertType,
		"task_id": t.task.Id,
	}))

	return n, nil
}

func (t *taskTriggers) taskOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskSucceeded && !isFailedTaskStatus(t.data.Status) {
		return nil, nil
	}

	return t.generate(sub, "", "")
}

func (t *taskTriggers) taskFailure(sub *event.Subscription) (*notification.Notification, error) {
	if !matchingFailureType(sub.TriggerData[keyFailureType], t.task.Details.Type) {
		return nil, nil
	}
	if !isFailedTaskStatus(t.data.Status) {
		return nil, nil
	}

	return t.generate(sub, "", "")
}

func (t *taskTriggers) taskSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	return t.generate(sub, "", "")
}

func (t *taskTriggers) taskFirstFailureInBuild(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskFailed {
		return nil, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInVariant(sub.ID, t.task.Version, t.task.BuildVariant))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", triggerTaskFirstFailureInBuild))
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(sub, alertrecord.FirstVariantFailureId, "", "")
}

func (t *taskTriggers) taskFirstFailureInVersion(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskFailed {
		return nil, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInVersion(sub.ID, t.task.Project, t.task.Version))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", triggerTaskFirstFailureInVersion))
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(sub, alertrecord.FirstVersionFailureId, "", "")
}

func (t *taskTriggers) taskFirstFailureInVersionWithName(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskFailed {
		return nil, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInTaskType(sub.ID, t.task.Version, t.task.DisplayName))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", triggerTaskFirstFailureInVersionWithName))
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(sub, alertrecord.FirstTaskTypeFailureId, "", "")
}

func (t *taskTriggers) taskRegression(sub *event.Subscription) (*notification.Notification, error) {
	shouldNotify, alert, err := isTaskRegression(sub, t.task)
	if err != nil {
		return nil, err
	}
	if !shouldNotify {
		return nil, nil
	}
	n, err := t.generate(sub, "", "")
	if err != nil {
		return nil, err
	}
	return n, errors.Wrap(alert.Insert(), "failed to process regression trigger")
}

func isTaskRegression(sub *event.Subscription, t *task.Task) (bool, *alertrecord.AlertRecord, error) {
	if t.Status != evergreen.TaskFailed || !util.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.Requester) {
		return false, nil, nil
	}

	// Regressions are not actionable if they're caused by a host that was terminated or an agent that died
	if t.Details.Description == evergreen.TaskDescriptionStranded || t.Details.Description == evergreen.TaskDescriptionHeartbeat {
		return false, nil, nil
	}

	if !matchingFailureType(sub.TriggerData[keyFailureType], t.Details.Type) {
		return false, nil, nil
	}

	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatusesAndRequesters(t.RevisionOrderNumber,
		task.CompletedStatuses, t.BuildVariant, t.DisplayName, t.Project, evergreen.SystemVersionRequesterTypes))
	if err != nil {
		return false, nil, errors.Wrap(err, "error fetching previous task")
	}

	shouldSend, err := shouldSendTaskRegression(sub, t, previousTask)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to determine if we should send notification")
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

func shouldSendTaskRegression(sub *event.Subscription, t *task.Task, previousTask *task.Task) (bool, error) {
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
		lastAlerted, err := alertrecord.FindOne(q)
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
		lastAlerted, err := alertrecord.FindOne(q)
		if err != nil {
			errMessage := getShouldExecuteError(t, previousTask)
			errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
			if err != nil {
				errMessage["error"] = err.Error()
			}
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

		return taskFinishedTwoOrMoreDaysAgo(lastAlerted.TaskId, sub)
	}
	return false, nil
}

func (t *taskTriggers) taskExceedsDuration(sub *event.Subscription) (*notification.Notification, error) {

	thresholdString, ok := sub.TriggerData[event.TaskDurationKey]
	if !ok {
		return nil, fmt.Errorf("subscription %s has no task time threshold", sub.ID)
	}
	threshold, err := strconv.Atoi(thresholdString)
	if err != nil {
		return nil, fmt.Errorf("subscription %s has an invalid time threshold", sub.ID)
	}

	maxDuration := time.Duration(threshold) * time.Second
	if !t.task.StartTime.Add(maxDuration).Before(t.task.FinishTime) {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("exceeded %d seconds", threshold), "")
}

func (t *taskTriggers) taskRuntimeChange(sub *event.Subscription) (*notification.Notification, error) {
	if t.task.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	percentString, ok := sub.TriggerData[event.TaskPercentChangeKey]
	if !ok {
		return nil, fmt.Errorf("subscription %s has no percentage increase", sub.ID)
	}
	percent, err := strconv.ParseFloat(percentString, 64)
	if err != nil {
		return nil, fmt.Errorf("subscription %s has an invalid percentage", sub.ID)
	}

	lastGreen, err := t.task.PreviousCompletedTask(t.task.Project, []string{evergreen.TaskSucceeded})
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving last green task")
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
	return t.generate(sub, fmt.Sprintf("changed in runtime by %.1f%% (over threshold of %s%%)", percentChange, percentString), "")
}

func isFailedTaskStatus(status string) bool {
	return status == evergreen.TaskFailed || status == evergreen.TaskSystemFailed || status == evergreen.TaskTestTimedOut
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

func (t *taskTriggers) shouldIncludeTest(sub *event.Subscription, previousTask *task.Task, test *task.TestResult) (bool, error) {
	if test.Status != evergreen.TestFailedStatus {
		return false, nil
	}
	record, err := alertrecord.FindByLastTaskRegressionByTest(sub.ID, test.TestFile, t.task.DisplayName, t.task.BuildVariant, t.task.Project)
	if err != nil {
		return false, errors.Wrap(err, "Failed to fetch alert record")
	}
	if record != nil {
		if record.RevisionOrderNumber == t.task.RevisionOrderNumber || previousTask == nil {
			return false, nil
		}

		oldTestResult, ok := t.oldTestResults[test.TestFile]
		if !ok {
			if test.Status != evergreen.TestFailedStatus {
				return false, nil
			}

		} else if !isTestStatusRegression(oldTestResult.Status, test.Status) {
			if len(record.TaskId) != 0 {
				var isOld bool
				isOld, err = taskFinishedTwoOrMoreDaysAgo(record.TaskId, sub)
				if err != nil || !isOld {
					return false, errors.Wrap(err, "failed to fetch last alert age")
				}
			}
		}
	}

	err = alertrecord.InsertNewTaskRegressionByTestRecord(sub.ID, t.task.Id, test.TestFile, t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
	if err != nil {
		return false, errors.Wrap(err, "failed to save alert record")
	}
	return true, nil
}

func (t *taskTriggers) taskRegressionByTest(sub *event.Subscription) (*notification.Notification, error) {
	if !util.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.task.Requester) || !isFailedTaskStatus(t.task.Status) {
		return nil, nil
	}
	if !matchingFailureType(sub.TriggerData[keyFailureType], t.task.Details.Type) {
		return nil, nil
	}
	// if no tests, alert only if it's a regression in task status
	if len(t.task.LocalTestResults) == 0 {
		return t.taskRegression(sub)
	}

	catcher := grip.NewBasicCatcher()
	previousCompleteTask, err := task.FindOne(task.ByBeforeRevisionWithStatusesAndRequesters(t.task.RevisionOrderNumber,
		task.CompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project, evergreen.SystemVersionRequesterTypes))
	if err != nil {
		return nil, errors.Wrap(err, "error fetching previous task")
	}

	if previousCompleteTask != nil {
		t.oldTestResults = mapTestResultsByTestFile(previousCompleteTask)
	}

	testsToAlert := []task.TestResult{}
	var match bool
	for i := range t.task.LocalTestResults {
		match, err = testMatchesRegex(t.task.LocalTestResults[i].TestFile, sub)
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
		shouldInclude, err = t.shouldIncludeTest(sub, previousCompleteTask, &t.task.LocalTestResults[i])
		if err != nil {
			catcher.Add(err)
			continue
		}
		if shouldInclude {
			testsToAlert = append(testsToAlert, t.task.LocalTestResults[i])
		}
	}
	if len(testsToAlert) == 0 {
		return nil, nil
	}
	testNames := ""
	for i, test := range testsToAlert {
		testNames += test.TestFile
		if i != len(testsToAlert)-1 {
			testNames += ", "
		}
	}

	n, err := t.generate(sub, "", testNames)
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

func (j *taskTriggers) makeJIRATaskPayload(subID, project, testNames string) (*message.JiraIssue, error) {
	return JIRATaskPayload(subID, project, j.uiConfig.Url, j.event.ID, testNames, j.task)
}

func JIRATaskPayload(subID, project, uiUrl, eventID, testNames string, t *task.Task) (*message.JiraIssue, error) {
	buildDoc, err := build.FindOne(build.ById(t.BuildId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch build while building jira task payload")
	}
	if buildDoc == nil {
		return nil, errors.New("could not find build while building jira task payload")
	}

	var hostDoc *host.Host
	if len(t.HostId) != 0 {
		hostDoc, err = host.FindOneId(t.HostId)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch host while building jira task payload")
		}
	}

	versionDoc, err := model.VersionFindOneId(t.Version)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch version while building jira task payload")
	}
	if versionDoc == nil {
		return nil, errors.New("could not find version while building jira task payload")
	}

	projectRef, err := model.FindOneProjectRef(t.Project)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch project ref while building jira task payload")
	}
	if projectRef == nil {
		return nil, errors.New("could not find project ref while building jira task payload")
	}

	data := jiraTemplateData{
		UIRoot:          uiUrl,
		SubscriptionID:  subID,
		EventID:         eventID,
		Task:            t,
		Version:         versionDoc,
		Project:         projectRef,
		Build:           buildDoc,
		Host:            hostDoc,
		TaskDisplayName: t.DisplayName,
	}
	if t.IsPartOfDisplay() {
		data.TaskDisplayName = t.DisplayTask.DisplayName
	}

	builder := jiraBuilder{
		project:  strings.ToUpper(project),
		mappings: &evergreen.JIRANotificationsConfig{},
		data:     data,
	}

	if err = builder.mappings.Get(evergreen.GetEnvironment()); err != nil {
		return nil, errors.Wrap(err, "failed to fetch jira custom field mappings while building jira task payload")
	}

	return builder.build()
}

// mapTestResultsByTestFile creates map of test file to TestResult struct. If
// multiple tests of the same name exist, this function will return a
// failing test if one existed, otherwise it may return any test with
// the same name
func mapTestResultsByTestFile(t *task.Task) map[string]*task.TestResult {
	m := map[string]*task.TestResult{}

	for i := range t.LocalTestResults {
		if testResult, ok := m[t.LocalTestResults[i].TestFile]; ok {
			if !isTestStatusRegression(testResult.Status, t.LocalTestResults[i].Status) {
				continue
			}
		}
		m[t.LocalTestResults[i].TestFile] = &t.LocalTestResults[i]
	}

	return m
}

func taskFormat(t *task.Task) string {
	if t.Status == evergreen.TaskSucceeded {
		return fmt.Sprintf("took %s", t.ExpectedDuration)
	}

	return fmt.Sprintf("took %s, the task failed %s", t.ExpectedDuration, detailStatusToHumanSpeak(t.Details.Status))
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
	default:
		return fmt.Sprintf("because of something else (%s)", status)
	}
}

// this is very similar to taskRegression, but different enough
func (t *taskTriggers) buildBreak(sub *event.Subscription) (*notification.Notification, error) {
	if t.task.Status != evergreen.TaskFailed || !util.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.task.Requester) {
		return nil, nil
	}

	// Regressions are not actionable if they're caused by a host that was terminated or an agent that died
	if t.task.Details.Description == evergreen.TaskDescriptionStranded || t.task.Details.Description == evergreen.TaskDescriptionHeartbeat {
		return nil, nil
	}

	if t.task.TriggerID != "" && sub.Owner != "" { // don't notify committer for a triggered build
		return nil, nil
	}
	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatusesAndRequesters(t.task.RevisionOrderNumber,
		task.CompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project, evergreen.SystemVersionRequesterTypes))
	if err != nil {
		return nil, errors.Wrap(err, "error fetching previous task")
	}
	if previousTask != nil && previousTask.Status == evergreen.TaskFailed {
		return nil, nil
	}

	lastAlert, err := alertrecord.FindByFirstRegressionInVersion(sub.ID, t.task.Version)
	if err != nil {
		return nil, errors.Wrap(err, "error finding last alert")
	}
	if lastAlert != nil {
		return nil, nil
	}

	n, err := t.generateWithAlertRecord(sub, alertrecord.FirstRegressionInVersion, "caused a regression", "")
	if err != nil {
		return nil, err
	}
	return n, nil
}
