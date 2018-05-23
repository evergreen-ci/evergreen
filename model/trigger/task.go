package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

func init() {
	registry.AddTrigger(event.ResourceTypeTask,
		taskValidator(taskOutcome),
		taskValidator(taskFailure),
		taskValidator(taskSuccess),
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

func taskOutcome(e *event.TaskEventData, t *task.Task) (*notificationGenerator, error) {
	const name = "outcome"

	if e.Status != evergreen.PatchSucceeded && e.Status != evergreen.PatchFailed {
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
