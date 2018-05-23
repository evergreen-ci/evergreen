package trigger

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	registry.AddTrigger(event.ResourceTypeTask,
		taskValidator(taskOutcome),
		taskValidator(taskFailure),
		taskValidator(taskSuccess),
		taskValidator(taskFirstFailureInBuild),
		taskValidator(taskFirstFailureInVersion),
		taskValidator(taskFirstFailureInVersionWithName),
		taskValidator(taskRegression),
	)
	registry.AddPrefetch(event.ResourceTypeTask, taskFetch)
}

func taskFetch(e *event.EventLogEntry) (interface{}, error) {
	p, err := task.FindOne(task.ById(e.ResourceId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch task")
	}
	if p == nil {
		return nil, errors.New("couldn't find task")
	}

	return p, nil
}

func taskValidator(triggerFunc func(*event.TaskEventData, *task.Task) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		t, ok := object.(*task.Task)
		if !ok {
			return nil, errors.New("expected a task, received unknown type")
		}
		if t == nil {
			return nil, errors.New("expected a task, received nil data")
		}

		data, ok := e.Data.(*event.TaskEventData)
		if !ok {
			return nil, errors.New("expected task event data")
		}

		return triggerFunc(data, t)
	}
}

func taskSelectors(t *task.Task) []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.Id,
		},
		{
			Type: selectorObject,
			Data: "task",
		},
		{
			Type: selectorProject,
			Data: t.Project,
		},
		{
			Type: selectorInVersion,
			Data: t.Version,
		},
		{
			Type: selectorInBuild,
			Data: t.BuildId,
		},
		{
			Type: selectorRequester,
			Data: t.Requester,
		},
	}
}

func generatorFromTask(triggerName string, t *task.Task, status string) (*notificationGenerator, error) {
	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	api := restModel.APITask{}
	if err := api.BuildFromService(t); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	selectors := taskSelectors(t)

	data := commonTemplateData{
		ID:              t.Id,
		Object:          "task",
		Project:         t.Project,
		URL:             fmt.Sprintf("%s/task/%s", ui.Url, t.Id),
		PastTenseStatus: status,
		apiModel:        &api,
	}
	if data.PastTenseStatus == evergreen.TaskSucceeded {
		data.PastTenseStatus = "succeeded"
	}

	return makeCommonGenerator(triggerName, selectors, data)
}

func generatorFromTaskWithAlertRecord(triggerName string, t *task.Task, status, alertType string) (*notificationGenerator, error) {
	gen, err := generatorFromTask(triggerName, t, status)
	if err != nil {
		return nil, err
	}

	rec := newAlertRecord(t, alertType)
	grip.Error(message.WrapError(rec.Insert(), message.Fields{
		"source":  "alert-record",
		"type":    alertType,
		"task_id": t.Id,
	}))

	return gen, nil
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

func taskOutcome(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "outcome"

	if e.Status != evergreen.TaskSucceeded && e.Status != evergreen.TaskFailed {
		return nil, nil
	}

	return generatorFromTask(name, t, e.Status)
}

func taskFailure(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "failure"

	if e.Status != evergreen.TaskFailed {
		return nil, nil
	}

	return generatorFromTask(name, t, e.Status)
}

func taskSuccess(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "success"

	if e.Status != evergreen.TaskSucceeded {
		return nil, nil
	}

	return generatorFromTask(name, t, e.Status)
}

func taskFirstFailureInBuild(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "first-failure-in-build"

	if e.Status != evergreen.TaskFailed {
		return nil, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInVariant(t.Version, t.BuildVariant))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", name))
	}
	if rec != nil {
		return nil, nil
	}

	return generatorFromTaskWithAlertRecord(name, t, e.Status, alertrecord.FirstVariantFailureId)
}

func taskFirstFailureInVersion(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "first-failure-in-version"

	if e.Status != evergreen.TaskFailed {
		return nil, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInVersion(t.Project, t.Version))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", name))
	}
	if rec != nil {
		return nil, nil
	}

	return generatorFromTaskWithAlertRecord(name, t, e.Status, alertrecord.FirstVersionFailureId)
}

func taskFirstFailureInVersionWithName(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "first-failure-in-version-with-name"

	if e.Status != evergreen.TaskFailed {
		return nil, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByFirstFailureInTaskType(t.Version, t.DisplayName))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to fetch alertrecord (%s)", name))
	}
	if rec != nil {
		return nil, nil
	}

	return generatorFromTaskWithAlertRecord(name, t, e.Status, alertrecord.FirstTaskTypeFailureId)
}

func taskRegression(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "regression"

	if e.Status != evergreen.TaskFailed {
		return nil, nil
	}

	previousTask, err := task.FindOne(task.ByBeforeRevisionWithStatuses(t.RevisionOrderNumber,
		task.CompletedStatuses, t.BuildVariant, t.DisplayName, t.Project))
	if err != nil {
		return nil, errors.Wrap(err, "error fetching previous task")

	}
	if previousTask != nil {
		q := alertrecord.ByLastFailureTransition(t.DisplayName, t.BuildVariant, t.Project)
		lastAlerted, err := alertrecord.FindOne(q)
		if err != nil {
			errMessage := getShouldExecuteError(t, previousTask)
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
				errMessage := getShouldExecuteError(t, previousTask)
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
					return nil, nil
				}

			} else if old, err := taskFinishedTwoOrMoreDaysAgo(lastAlerted.TaskId); !old {
				return nil, errors.Wrap(err, "failed to process regression trigger")
			}
		}
	}

	gen, err := generatorFromTask(name, t, e.Status)
	if err != nil {
		return gen, err
	}

	rec := newAlertRecord(t, alertrecord.TaskFailTransitionId)
	rec.RevisionOrderNumber = -1
	if previousTask != nil {
		rec.RevisionOrderNumber = previousTask.RevisionOrderNumber
	}

	return gen, errors.Wrap(rec.Insert(), "failed to process regression trigger")
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
	}

	if previousTask == nil {
		m["previous"] = nil
	} else {
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
