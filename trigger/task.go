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
	"github.com/evergreen-ci/evergreen/model/version"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeTask, event.TaskFinished, makeTaskTriggers)
}

const (
	objectTask                               = "task"
	triggerTaskFirstFailureInBuild           = "first-failure-in-build"
	triggerTaskFirstFailureInVersion         = "first-failure-in-version"
	triggerTaskFirstFailureInVersionWithName = "first-failure-in-version-with-name"
	triggerTaskRegressionByTest              = "regression-by-test"
	triggerBuildBreak                        = "build-break"
)

func makeTaskTriggers() eventHandler {
	t := &taskTriggers{
		oldTestResults: map[string]*task.TestResult{},
	}
	t.base.triggers = map[string]trigger{
		triggerOutcome:                           t.taskOutcome,
		triggerFailure:                           t.taskFailure,
		triggerSuccess:                           t.taskSuccess,
		triggerTaskFirstFailureInBuild:           t.taskFirstFailureInBuild,
		triggerTaskFirstFailureInVersion:         t.taskFirstFailureInVersion,
		triggerTaskFirstFailureInVersionWithName: t.taskFirstFailureInVersionWithName,
		triggerExceedsDuration:                   t.taskExceedsDuration,
		triggerRuntimeChangeByPercent:            t.taskRuntimeChange,
		triggerRegression:                        t.taskRegression,
		triggerTaskRegressionByTest:              t.taskRegressionByTest,
		triggerBuildBreak:                        t.buildBreak,
	}

	return t
}

// newAlertRecord creates an instance of an alert record for the given alert type, populating it
// with as much data from the triggerContext as possible
func newAlertRecord(subID string, t *task.Task, alertType string) *alertrecord.AlertRecord {
	return &alertrecord.AlertRecord{
		Id:                  bson.NewObjectId(),
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

func taskFinishedTwoOrMoreDaysAgo(taskID string) (bool, error) {
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

	return time.Since(t.FinishTime) >= 48*time.Hour, nil
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
	version  *version.Version
	uiConfig evergreen.UIConfig

	oldTestResults map[string]*task.TestResult

	base
}

func (t *taskTriggers) Fetch(e *event.EventLogEntry) error {
	var ok bool
	t.data, ok = e.Data.(*event.TaskEventData)
	if !ok {
		return errors.New("expected task event data")
	}

	var err error
	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.task, err = task.FindOneIdOldOrNew(e.ResourceId, t.data.Execution)
	if err != nil {
		return errors.Wrap(err, "failed to fetch task")
	}
	if t.task == nil {
		return errors.New("couldn't find task")
	}
	if t.task.DisplayOnly {
		err = t.task.MergeNewTestResults()
		if err != nil {
			return errors.Wrap(err, "error getting test results")
		}
	}

	t.version, err = version.FindOne(version.ById(t.task.Version))
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
			Type: selectorID,
			Data: t.task.Id,
		},
		{
			Type: selectorObject,
			Data: objectTask,
		},
		{
			Type: selectorProject,
			Data: t.task.Project,
		},
		{
			Type: selectorInVersion,
			Data: t.task.Version,
		},
		{
			Type: selectorInBuild,
			Data: t.task.BuildId,
		},
		{
			Type: selectorRequester,
			Data: t.task.Requester,
		},
		{
			Type: selectorDisplayName,
			Data: t.task.DisplayName,
		},
	}
	if t.version != nil && t.version.AuthorID != "" {
		selectors = append(selectors, event.Selector{
			Type: selectorOwner,
			Data: t.version.AuthorID,
		})
	}

	return selectors
}

func (t *taskTriggers) makeData(sub *event.Subscription, pastTenseOverride string) (*commonTemplateData, error) {
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

	data := commonTemplateData{
		ID:              t.task.Id,
		EventID:         t.event.ID,
		SubscriptionID:  sub.ID,
		DisplayName:     t.task.DisplayName,
		Object:          "task",
		Project:         t.task.Project,
		URL:             taskLink(&t.uiConfig, t.task.Id, t.task.Execution),
		PastTenseStatus: t.data.Status,
		apiModel:        &api,
		Task:            t.task,
		ProjectRef:      projectRef,
		Build:           buildDoc,
	}
	slackColor := evergreenFailColor

	if len(t.task.OldTaskId) != 0 {
		data.URL = taskLink(&t.uiConfig, t.task.OldTaskId, t.task.Execution)
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
			Title:     t.task.DisplayName,
			TitleLink: data.URL,
			Text:      taskFormat(t.task),
			Color:     slackColor,
		},
	}

	return &data, nil
}

func (t *taskTriggers) generate(sub *event.Subscription, pastTenseOverride string) (*notification.Notification, error) {
	var payload interface{}
	if sub.Subscriber.Type == event.JIRAIssueSubscriberType {
		issueSub, ok := sub.Subscriber.Target.(*event.JIRAIssueSubscriber)
		if !ok {
			return nil, errors.Errorf("unexpected target data type: '%T'", sub.Subscriber.Target)
		}
		var err error
		payload, err = t.makeJIRATaskPayload(sub.ID, issueSub.Project)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create jira payload for task")
		}

	} else {
		data, err := t.makeData(sub, pastTenseOverride)
		if err != nil {
			return nil, errors.Wrap(err, "failed to collect task data")
		}
		data.emailContent = emailTaskContentTemplate

		payload, err = makeCommonPayload(sub, t.Selectors(), data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build notification")
		}
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}

func (t *taskTriggers) generateWithAlertRecord(sub *event.Subscription, alertType, pastTenseOverride string) (*notification.Notification, error) {
	n, err := t.generate(sub, pastTenseOverride)
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

	return t.generate(sub, "")
}

func (t *taskTriggers) taskFailure(sub *event.Subscription) (*notification.Notification, error) {
	if !isFailedTaskStatus(t.data.Status) {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *taskTriggers) taskSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	return t.generate(sub, "")
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

	return t.generateWithAlertRecord(sub, alertrecord.FirstVariantFailureId, "")
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

	return t.generateWithAlertRecord(sub, alertrecord.FirstVersionFailureId, "")
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

	return t.generateWithAlertRecord(sub, alertrecord.FirstTaskTypeFailureId, "")
}

func (t *taskTriggers) taskRegression(sub *event.Subscription) (*notification.Notification, error) {
	shouldNotify, alert, err := isTaskRegression(sub.ID, t.task)
	if err != nil {
		return nil, err
	}
	if !shouldNotify {
		return nil, nil
	}
	n, err := t.generate(sub, "")
	if err != nil {
		return nil, err
	}
	return n, errors.Wrap(alert.Insert(), "failed to process regression trigger")
}

func isTaskRegression(subID string, t *task.Task) (bool, *alertrecord.AlertRecord, error) {
	if t.Status != evergreen.TaskFailed || t.Requester != evergreen.RepotrackerVersionRequester {
		return false, nil, nil
	}

	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatuses(t.RevisionOrderNumber,
		task.CompletedStatuses, t.BuildVariant, t.DisplayName, t.Project))
	if err != nil {
		return false, nil, errors.Wrap(err, "error fetching previous task")
	}

	shouldSend, err := shouldSendTaskRegression(subID, t, previousTask)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to determine if we should send notification")
	}
	if !shouldSend {
		return false, nil, nil
	}

	rec := newAlertRecord(subID, t, alertrecord.TaskFailTransitionId)
	rec.RevisionOrderNumber = -1
	if previousTask != nil {
		rec.RevisionOrderNumber = previousTask.RevisionOrderNumber
	}

	return true, rec, nil
}

func shouldSendTaskRegression(subID string, t *task.Task, previousTask *task.Task) (bool, error) {
	if t.Status != evergreen.TaskFailed {
		return false, nil
	}
	if previousTask == nil {
		return true, nil
	}

	if previousTask.Status == evergreen.TaskSucceeded {
		// the task transitioned to failure - but we will only trigger an alert if we haven't recorded
		// a sent alert for a transition after the same previously passing task.
		q := alertrecord.ByLastFailureTransition(subID, t.DisplayName, t.BuildVariant, t.Project)
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
		q := alertrecord.ByLastFailureTransition(subID, t.DisplayName, t.BuildVariant, t.Project)
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

		return taskFinishedTwoOrMoreDaysAgo(lastAlerted.TaskId)
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
	return t.generate(sub, fmt.Sprintf("exceeded %d seconds", threshold))
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
	return t.generate(sub, fmt.Sprintf("changed in runtime by %.1f%% (over threshold of %s%%)", percentChange, percentString))
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

func isTaskStatusRegression(oldStatus, newStatus string) bool {
	switch oldStatus {
	case evergreen.TaskFailed:
		if newStatus == evergreen.TaskSystemFailed || newStatus == evergreen.TaskTestTimedOut {
			return true
		}

	case evergreen.TaskSystemFailed:
		if newStatus == evergreen.TaskFailed || newStatus == evergreen.TaskTestTimedOut {
			return true
		}

	case evergreen.TaskSucceeded:
		return isFailedTaskStatus(newStatus)
	}

	return false
}

func testMatchesRegex(testName string, sub *event.Subscription) bool {
	regex, ok := sub.TriggerData[event.TestRegexKey]
	if !ok || regex == "" {
		return true
	}
	match, err := regexp.MatchString(regex, testName)
	grip.Error(message.WrapError(err, message.Fields{
		"source":  "test-trigger",
		"message": "bad regex in db",
	}))
	if match {
		return true
	}
	return false
}

func (t *taskTriggers) shouldIncludeTest(subID string, previousTask *task.Task, test *task.TestResult) (bool, error) {
	if test.Status != evergreen.TestFailedStatus {
		return false, nil
	}
	record, err := alertrecord.FindByLastTaskRegressionByTest(subID, test.TestFile, t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
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
				isOld, err := taskFinishedTwoOrMoreDaysAgo(record.TaskId)
				if err != nil || !isOld {
					return false, errors.Wrap(err, "failed to fetch last alert age")
				}
			}
		}
	}

	err = alertrecord.InsertNewTaskRegressionByTestRecord(subID, t.task.Id, test.TestFile, t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
	if err != nil {
		return false, errors.Wrap(err, "failed to save alert record")
	}
	return true, nil
}

func (t *taskTriggers) taskRegressionByTest(sub *event.Subscription) (*notification.Notification, error) {
	if t.task.Requester != evergreen.RepotrackerVersionRequester || !isFailedTaskStatus(t.task.Status) {
		return nil, nil
	}

	previousCompleteTask, err := task.FindOne(task.ByBeforeRevisionWithStatuses(t.task.RevisionOrderNumber,
		task.CompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project))
	if err != nil {
		return nil, errors.Wrap(err, "error fetching previous task")
	}

	record, err := alertrecord.FindByLastTaskRegressionByTestWithNoTests(sub.ID, t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch alertrecord")
	}

	catcher := grip.NewBasicCatcher()
	// if no tests, alert only if it's a regression in task status
	if len(t.task.LocalTestResults) == 0 {
		if record != nil && !isTaskStatusRegression(record.TaskStatus, t.task.Status) {
			return nil, nil
		}
		catcher.Add(alertrecord.InsertNewTaskRegressionByTestWithNoTestsRecord(sub.ID, t.task.Id, t.task.DisplayName, t.task.Status, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber))

	} else {
		if previousCompleteTask != nil {
			t.oldTestResults = mapTestResultsByTestFile(previousCompleteTask)
		}

		testsToAlert := []task.TestResult{}
		for i := range t.task.LocalTestResults {
			if !testMatchesRegex(t.task.LocalTestResults[i].TestFile, sub) {
				continue
			}
			var shouldInclude bool
			shouldInclude, err = t.shouldIncludeTest(sub.ID, previousCompleteTask, &t.task.LocalTestResults[i])
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
		// TODO EVG-3416 use testsToAlert in message formatting
	}

	n, err := t.generate(sub, "")
	if err != nil {
		return nil, err
	}

	return n, catcher.Resolve()
}

func (j *taskTriggers) makeJIRATaskPayload(subID, project string) (*message.JiraIssue, error) {
	buildDoc, err := build.FindOne(build.ById(j.task.BuildId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch build while building jira task payload")
	}
	if buildDoc == nil {
		return nil, errors.New("could not find build while building jira task payload")
	}

	var hostDoc *host.Host
	if len(j.task.HostId) != 0 {
		hostDoc, err = host.FindOneId(j.task.HostId)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch host while building jira task payload")
		}
	}

	versionDoc, err := version.FindOneId(j.task.Version)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch version while building jira task payload")
	}
	if versionDoc == nil {
		return nil, errors.New("could not find version while building jira task payload")
	}

	projectRef, err := model.FindOneProjectRef(j.task.Project)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch project ref while building jira task payload")
	}
	if projectRef == nil {
		return nil, errors.New("could not find project ref while building jira task payload")
	}

	builder := jiraBuilder{
		project:  strings.ToUpper(project),
		mappings: &evergreen.JIRANotificationsConfig{},
		data: jiraTemplateData{
			UIRoot:         j.uiConfig.Url,
			SubscriptionID: subID,
			EventID:        j.event.ID,
			Task:           j.task,
			Version:        versionDoc,
			Project:        projectRef,
			Build:          buildDoc,
			Host:           hostDoc,
		},
	}

	if err = builder.mappings.Get(); err != nil {
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
	if t.task.Status != evergreen.TaskFailed || t.task.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}
	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatuses(t.task.RevisionOrderNumber,
		task.CompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project))
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

	n, err := t.generateWithAlertRecord(sub, alertrecord.FirstRegressionInVersion, "caused a regression")
	if err != nil {
		return nil, err
	}
	return n, nil
}
