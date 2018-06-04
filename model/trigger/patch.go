package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	registry.registerEventHandler(event.ResourceTypePatch, event.PatchStateChange, makePatchTriggers)
}

const (
	triggerPatchOutcome = "outcome"
	triggerPatchFailure = "failure"
	triggerPatchSuccess = "success"
	triggerPatchStarted = "started"
)

type patchTrigger func(*event.Subscription) (*notification.Notification, error)

type patchTriggers struct {
	event    *event.EventLogEntry
	data     *event.PatchEventData
	patch    *patch.Patch
	uiConfig evergreen.UIConfig

	triggers map[string]patchTrigger
}

func makePatchTriggers() eventHandler {
	t := &patchTriggers{}
	t.triggers = map[string]patchTrigger{
		triggerPatchOutcome: t.patchOutcome,
		triggerPatchFailure: t.patchFailure,
		triggerPatchSuccess: t.patchSuccess,
		triggerPatchStarted: t.patchStarted,
	}
	return t
}

func (t *patchTriggers) Fetch(e *event.EventLogEntry) error {
	var err error

	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}
	t.patch, err = patch.FindOne(patch.ById(bson.ObjectIdHex(e.ResourceId)))
	if err != nil {
		return errors.Wrapf(err, "failed to fetch patch '%s'", e.ResourceId)
	}
	if t.patch == nil {
		return errors.Wrapf(err, "can't find patch '%s'", e.ResourceId)
	}
	var ok bool
	t.data, ok = e.Data.(*event.PatchEventData)
	if !ok {
		return errors.Wrapf(err, "patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *patchTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.patch.Id.Hex(),
		},
		{
			Type: selectorObject,
			Data: "patch",
		},
		{
			Type: selectorProject,
			Data: t.patch.Project,
		},
		{
			Type: selectorOwner,
			Data: t.patch.Author,
		},
		{
			Type: selectorStatus,
			Data: t.patch.Status,
		},
	}
}

func (t *patchTriggers) ValidateTrigger(trigger string) bool {
	_, ok := t.triggers[trigger]
	return ok
}

func (t *patchTriggers) Process(sub *event.Subscription) (*notification.Notification, error) {
	f, ok := t.triggers[sub.Trigger]
	if !ok {
		return nil, errors.Errorf("unknown trigger: '%s'", sub.Trigger)
	}

	return f(sub)
}

func (t *patchTriggers) patchOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchStarted {
		return nil, nil
	}
	if t.data.Status != evergreen.PatchSucceeded && t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchStarted(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchStarted {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	api := restModel.APIPatch{}
	if err := api.BuildFromService(*t.patch); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:                t.patch.Id.Hex(),
		Object:            "patch",
		Project:           t.patch.Project,
		URL:               fmt.Sprintf("%s/version/%s", t.uiConfig.Url, t.patch.Version),
		PastTenseStatus:   t.data.Status,
		apiModel:          &api,
		githubState:       message.GithubStatePending,
		githubContext:     "evergreen",
		githubDescription: "tasks are running",
	}
	if t.data.Status == evergreen.PatchSucceeded {
		data.githubState = message.GithubStateSuccess
		data.githubDescription = fmt.Sprintf("patch finished in %s", t.patch.FinishTime.Sub(t.patch.StartTime).String())

	} else if t.data.Status == evergreen.PatchFailed {
		data.githubState = message.GithubStateFailure
		data.githubDescription = fmt.Sprintf("patch finished in %s", t.patch.FinishTime.Sub(t.patch.StartTime).String())
	}
	return &data, nil
}

func (t *patchTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect patch data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), *data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}
