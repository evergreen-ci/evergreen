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
	registry.AddTrigger(event.ResourceTypePatch,
		patchValidator(patchOutcome),
		patchValidator(patchFailure),
		patchValidator(patchSuccess),
		patchValidator(patchStarted),
	)
	registry.AddPrefetch(event.ResourceTypePatch, patchFetch)
}

const (
	triggerPatchOutcome = "outcome"
	triggerPatchFailure = "failure"
	triggerPatchSuccess = "success"
	triggerPatchStarted = "started"
)

func patchFetch(e *event.EventLogEntry) (interface{}, error) {
	p, err := patch.FindOne(patch.ById(bson.ObjectIdHex(e.ResourceId)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch patch")
	}
	if p == nil {
		return nil, errors.New("couldn't find patch")
	}

	return p, nil
}

func patchValidator(t func(e *event.PatchEventData, p *patch.Patch) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		p, ok := object.(*patch.Patch)
		if !ok {
			return nil, errors.New("expected a patch, received unknown type")
		}
		if p == nil {
			return nil, errors.New("expected a patch, received nil data")
		}

		data, ok := e.Data.(*event.PatchEventData)
		if !ok {
			return nil, errors.New("expected patch event data")
		}

		return t(data, p)
	}
}

func patchSelectors(p *patch.Patch) []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: p.Id.Hex(),
		},
		{
			Type: selectorObject,
			Data: "patch",
		},
		{
			Type: selectorProject,
			Data: p.Project,
		},
		{
			Type: selectorOwner,
			Data: p.Author,
		},
		{
			Type: selectorStatus,
			Data: p.Status,
		},
	}
}

func generatorFromPatch(triggerName string, p *patch.Patch, status string) (*notificationGenerator, error) {
	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	api := restModel.APIPatch{}
	if err := api.BuildFromService(*p); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	selectors := patchSelectors(p)

	data := commonTemplateData{
		ID:              p.Id.Hex(),
		Object:          "patch",
		Project:         p.Project,
		URL:             fmt.Sprintf("%s/version/%s", ui.Url, p.Version),
		PastTenseStatus: status,
		apiModel:        &api,
	}
	if status == p.Status {
		data.githubState = message.GithubStateFailure
		data.githubDescription = fmt.Sprintf("patch finished in %s", p.FinishTime.Sub(p.StartTime).String())

		if p.Status == evergreen.PatchSucceeded {
			data.githubState = message.GithubStateSuccess
		}
	}

	return makeCommonGenerator(triggerName, selectors, data)
}

func patchOutcome(e *event.PatchEventData, p *patch.Patch) (*notificationGenerator, error) {
	const name = "outcome"

	if e.Status != evergreen.PatchSucceeded && e.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return generatorFromPatch(name, p, e.Status)
}

func patchFailure(e *event.PatchEventData, p *patch.Patch) (*notificationGenerator, error) {
	const name = "failure"

	if e.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return generatorFromPatch(name, p, e.Status)
}

func patchSuccess(e *event.PatchEventData, p *patch.Patch) (*notificationGenerator, error) {
	const name = "success"

	if e.Status != evergreen.PatchSucceeded {
		return nil, nil
	}

	return generatorFromPatch(name, p, e.Status)
}

func patchStarted(e *event.PatchEventData, p *patch.Patch) (*notificationGenerator, error) {
	const name = "started"

	if e.Status != evergreen.PatchStarted {
		return nil, nil
	}

	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	gen := &notificationGenerator{
		triggerName: name,
		selectors:   patchSelectors(p),
		githubStatusAPI: &message.GithubStatus{
			Context:     "evergreen",
			State:       message.GithubStatePending,
			URL:         fmt.Sprintf("%s/version/%s", ui.Url, p.Id.Hex()),
			Description: "tasks are running",
		},
	}

	return gen, nil
}

type oldPatchTrigger func(e *event.PatchEventData, p *patch.Patch) (*notificationGenerator, error)

type patchTriggers struct {
	event *event.EventLogEntry
	data  *event.PatchEventData
	patch *patch.Patch

	triggers map[string]oldPatchTrigger
}

func makePatchTriggers() eventHandler {
	return &patchTriggers{
		triggers: map[string]oldPatchTrigger{
			triggerPatchOutcome: patchOutcome,
			triggerPatchFailure: patchFailure,
			triggerPatchSuccess: patchSuccess,
			triggerPatchStarted: patchStarted,
		},
	}
}

func (t *patchTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
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
	return patchSelectors(t.patch)
}

func (t *patchTriggers) Validate(trigger string) bool {
	_, ok := t.triggers[trigger]
	return ok
}

func (t *patchTriggers) Process(subscription *event.Subscription) (*notification.Notification, error) {
	f, ok := t.triggers[subscription.Trigger]
	if !ok {
		return nil, errors.Errorf("unknown trigger: '%s'", subscription.Trigger)
	}

	gen, err := f(t.data, t.patch)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to process trigger '%s'", subscription.Trigger)
	}
	if gen == nil {
		return nil, nil
	}

	payload, err := gen.get(subscription.Subscriber.Type)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to process trigger '%s'", subscription.Trigger)
	}
	if payload == nil {
		return nil, nil
	}

	n, err := notification.New(t.event, subscription.Trigger, &subscription.Subscriber, payload)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create notification fir trigger '%s'", subscription.Trigger)
	}

	return n, nil
}
