package trigger

import (
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
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
	}

	return t
}

// newAlertRecord creates an instance of an alert record for the given alert type, populating it
// with as much data from the triggerContext as possible
func newAlertRecord(t *task.Task, alertType string) *alertrecord.AlertRecord {
	return &alertrecord.AlertRecord{
		Id:                  bson.NewObjectId(),
		Type:                alertType,
		ProjectId:           t.Project,
		VersionId:           t.Version,
		RevisionOrderNumber: t.RevisionOrderNumber,
		TaskName:            t.DisplayName,
		Variant:             t.BuildVariant,
		TaskId:              t.Id,
		HostId:              t.HostId,
	}
}

func taskFinishedTwoOrMoreDaysAgo(taskID string) (bool, error) {
	t, err := task.FindOneNoMerge(task.ById(taskID))
	if err != nil {
		return false, err
	}
	if t == nil {
		return false, errors.Errorf("task %s not found", taskID)
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

	t.event = e

	return nil
}

func (t *taskTriggers) Selectors() []event.Selector {
	return []event.Selector{
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
}

func (t *taskTriggers) makeData(sub *event.Subscription, pastTenseOverride string) (*commonTemplateData, error) {
	api := restModel.APITask{}
	if err := api.BuildFromService(t.task); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:              t.task.Id,
		DisplayName:     t.task.DisplayName,
		Object:          "task",
		Project:         t.task.Project,
		URL:             taskLink(&t.uiConfig, t.task.Id, t.task.Execution),
		PastTenseStatus: t.data.Status,
		apiModel:        &api,
	}
	slackColor := evergreenFailColor

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
	data, err := t.makeData(sub, pastTenseOverride)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect task data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}

func (t *taskTriggers) generateWithAlertRecord(sub *event.Subscription, alertType, pastTenseOverride string) (*notification.Notification, error) {
	n, err := t.generate(sub, pastTenseOverride)
	if err != nil {
		return nil, err
	}

	rec := newAlertRecord(t.task, alertType)
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
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInVariant(t.task.Version, t.task.BuildVariant))
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
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInVersion(t.task.Project, t.task.Version))
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
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInTaskType(t.task.Version, t.task.DisplayName))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", triggerTaskFirstFailureInVersionWithName))
	}
	if rec != nil {
		return nil, nil
	}

	return t.generateWithAlertRecord(sub, alertrecord.FirstTaskTypeFailureId, "")
}

func (t *taskTriggers) taskRegression(sub *event.Subscription) (*notification.Notification, error) {
	shouldNotify, alert, err := isTaskRegression(t.task)
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

func isTaskRegression(t *task.Task) (bool, *alertrecord.AlertRecord, error) {
	if t.Status != evergreen.TaskFailed || t.Requester != evergreen.RepotrackerVersionRequester {
		return false, nil, nil
	}

	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatuses(t.RevisionOrderNumber,
		task.CompletedStatuses, t.BuildVariant, t.DisplayName, t.Project))
	if err != nil {
		return false, nil, errors.Wrap(err, "error fetching previous task")
	}
	if previousTask != nil {
		q := alertrecord.ByLastFailureTransition(t.DisplayName, t.BuildVariant, t.Project)
		var lastAlerted *alertrecord.AlertRecord
		lastAlerted, err = alertrecord.FindOne(q)
		if err != nil {
			errMessage := getShouldExecuteError(t, previousTask)
			errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
			errMessage["error"] = err.Error()
			grip.Error(errMessage)
			return false, nil, errors.Wrap(err, "failed to process regression trigger")
		}

		if previousTask.Status == evergreen.TaskSucceeded {
			// the task transitioned to failure - but we will only trigger an alert if we haven't recorded
			// a sent alert for a transition after the same previously passing task.
			if lastAlerted != nil && (lastAlerted.RevisionOrderNumber >= previousTask.RevisionOrderNumber) {
				return false, nil, nil
			}

		} else if previousTask.Status == evergreen.TaskFailed {
			if lastAlerted == nil {
				errMessage := getShouldExecuteError(t, previousTask)
				errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
				if err != nil {
					errMessage["error"] = err.Error()
				}
				errMessage["lastAlert"] = lastAlerted
				errMessage["outcome"] = "not sending alert"
				grip.Error(errMessage)
				return false, nil, errors.Wrap(err, "failed to process regression trigger")
			}

			// TODO: EVG-3407 how is this even possible?
			if lastAlerted.TaskId == "" {
				shouldSend := sometimes.Quarter()
				errMessage := getShouldExecuteError(t, previousTask)
				errMessage[message.FieldsMsgName] = "empty last alert task_id"
				errMessage["lastAlert"] = lastAlerted
				if shouldSend {
					errMessage["outcome"] = "sending alert (25%)"
				} else {
					errMessage["outcome"] = "not sending alert (75%)"
				}
				grip.Warning(errMessage)
				if !shouldSend {
					return false, nil, nil
				}

			} else {
				var old bool
				if old, err = taskFinishedTwoOrMoreDaysAgo(lastAlerted.TaskId); !old {
					return false, nil, errors.Wrap(err, "failed to process regression trigger")
				}
			}
		}
	}

	rec := newAlertRecord(t, alertrecord.TaskFailTransitionId)
	rec.RevisionOrderNumber = -1
	if previousTask != nil {
		rec.RevisionOrderNumber = previousTask.RevisionOrderNumber
	}

	return true, rec, nil
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

func (t *taskTriggers) shouldIncludeTest(previousTask *task.Task, test *task.TestResult) (bool, error) {
	if test.Status != evergreen.TestFailedStatus {
		return false, nil
	}
	record, err := alertrecord.FindByLastTaskRegressionByTest(test.TestFile, t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
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
			return false, nil
		}
	}

	err = alertrecord.InsertNewTaskRegressionByTestRecord(test.TestFile, t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
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

	record, err := alertrecord.FindByLastTaskRegressionByTestWithNoTests(t.task.DisplayName, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch alertrecord")
	}

	catcher := grip.NewBasicCatcher()
	// if no tests, alert only if it's a regression in task status
	if len(t.task.LocalTestResults) == 0 {
		if record != nil && !isTaskStatusRegression(record.TaskStatus, t.task.Status) {
			return nil, nil
		}
		catcher.Add(alertrecord.InsertNewTaskRegressionByTestWithNoTestsRecord(t.task.DisplayName, t.task.Status, t.task.BuildVariant, t.task.Project, t.task.RevisionOrderNumber))

	} else {
		if previousCompleteTask != nil {
			t.oldTestResults = mapTestResultsByTestFile(previousCompleteTask)
		}

		testsToAlert := []task.TestResult{}
		for i := range t.task.LocalTestResults {
			var shouldInclude bool
			shouldInclude, err = t.shouldIncludeTest(previousCompleteTask, &t.task.LocalTestResults[i])
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
