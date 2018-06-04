package trigger

import (
	"fmt"
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
	triggerTaskRegression                    = "regression"
)

func makeTaskTriggers() eventHandler {
	t := &taskTriggers{}
	t.base.triggers = map[string]trigger{
		triggerOutcome:                           t.taskOutcome,
		triggerFailure:                           t.taskFailure,
		triggerSuccess:                           t.taskSuccess,
		triggerTaskFirstFailureInBuild:           t.taskFirstFailureInBuild,
		triggerTaskFirstFailureInVersion:         t.taskFirstFailureInVersion,
		triggerTaskFirstFailureInVersionWithName: t.taskFirstFailureInVersionWithName,
		triggerTaskRegression:                    t.taskRegression,
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
	t, err := task.FindOne(task.ById(taskID))
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
	}
}

func (t *taskTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	api := restModel.APITask{}
	if err := api.BuildFromService(t.task); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:              t.task.Id,
		Object:          "task",
		Project:         t.task.Project,
		URL:             fmt.Sprintf("%s/task/%s", t.uiConfig.Url, t.task.Id),
		PastTenseStatus: t.data.Status,
		apiModel:        &api,
	}
	if data.PastTenseStatus == evergreen.TaskSucceeded {
		data.PastTenseStatus = "succeeded"
	}

	return &data, nil
}

func (t *taskTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect task data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), *data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}

func (t *taskTriggers) generateWithAlertRecord(sub *event.Subscription, alertType string) (*notification.Notification, error) {
	n, err := t.generate(sub)
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
	if t.data.Status != evergreen.TaskSucceeded && t.data.Status != evergreen.TaskFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *taskTriggers) taskFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *taskTriggers) taskSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	return t.generate(sub)
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

	return t.generateWithAlertRecord(sub, alertrecord.FirstVariantFailureId)
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

	return t.generateWithAlertRecord(sub, alertrecord.FirstVersionFailureId)
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

	return t.generateWithAlertRecord(sub, alertrecord.FirstTaskTypeFailureId)
}

func (t *taskTriggers) taskRegression(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.TaskFailed || t.task.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}

	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatuses(t.task.RevisionOrderNumber,
		task.CompletedStatuses, t.task.BuildVariant, t.task.DisplayName, t.task.Project))
	if err != nil {
		return nil, errors.Wrap(err, "error fetching previous task")

	}
	if previousTask != nil {
		q := alertrecord.ByLastFailureTransition(t.task.DisplayName, t.task.BuildVariant, t.task.Project)
		lastAlerted, err := alertrecord.FindOne(q)
		if err != nil {
			errMessage := getShouldExecuteError(t.task, previousTask)
			errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
			errMessage["error"] = err.Error()
			grip.Error(errMessage)
			return nil, errors.Wrap(err, "failed to process regression trigger")
		}

		if previousTask.Status == evergreen.TaskSucceeded {
			// the task transitioned to failure - but we will only trigger an alert if we haven't recorded
			// a sent alert for a transition after the same previously passing task.
			if lastAlerted != nil && (lastAlerted.RevisionOrderNumber >= previousTask.RevisionOrderNumber) {
				return nil, nil
			}

		} else if previousTask.Status == evergreen.TaskFailed {
			if lastAlerted == nil {
				errMessage := getShouldExecuteError(t.task, previousTask)
				errMessage[message.FieldsMsgName] = "could not find a record for the last alert"
				if err != nil {
					errMessage["error"] = err.Error()
				}
				errMessage["lastAlert"] = lastAlerted
				errMessage["outcome"] = "not sending alert"
				grip.Error(errMessage)
				return nil, errors.Wrap(err, "failed to process regression trigger")
			}

			// TODO: EVG-3407 how is this even possible?
			if lastAlerted.TaskId == "" {
				shouldSend := sometimes.Quarter()
				errMessage := getShouldExecuteError(t.task, previousTask)
				errMessage[message.FieldsMsgName] = "empty last alert task_id"
				errMessage["lastAlert"] = lastAlerted
				if shouldSend {
					errMessage["outcome"] = "sending alert (25%)"
				} else {
					errMessage["outcome"] = "not sending alert (75%)"
				}
				grip.Warning(errMessage)
				if !shouldSend {
					return nil, nil
				}

			} else if old, err := taskFinishedTwoOrMoreDaysAgo(lastAlerted.TaskId); !old {
				return nil, errors.Wrap(err, "failed to process regression trigger")
			}
		}
	}

	n, err := t.generate(sub)
	if err != nil {
		return nil, err
	}

	rec := newAlertRecord(t.task, alertrecord.TaskFailTransitionId)
	rec.RevisionOrderNumber = -1
	if previousTask != nil {
		rec.RevisionOrderNumber = previousTask.RevisionOrderNumber
	}

	return n, errors.Wrap(rec.Insert(), "failed to process regression trigger")
}
